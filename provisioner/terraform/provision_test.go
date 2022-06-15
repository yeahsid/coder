//go:build linux

package terraform_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProvision(t *testing.T) {
	t.Parallel()

	provider := `
terraform {
	required_providers {
		coder = {
			source = "coder/coder"
			version = "0.4.2"
		}
	}
}

provider "coder" {
}
	`
	t.Log(provider)

	client, server := provisionersdk.TransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		cancelFunc()
	})
	go func() {
		err := terraform.Serve(ctx, &terraform.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Listener: server,
			},
			Logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		})
		assert.NoError(t, err)
	}()
	api := proto.NewDRPCProvisionerClient(provisionersdk.Conn(client))

	testCases := []struct {
		Name    string
		Files   map[string]string
		Request *proto.Provision_Request
		// Response may be nil to not check the response.
		Response *proto.Provision_Response
		// If ErrorContains is not empty, then response.Recv() should return an
		// error containing this string before a Complete response is returned.
		ErrorContains string
		// If ExpectLogContains is not empty, then the logs should contain it.
		ExpectLogContains string
	}{
		{
			Name: "single-variable",
			Files: map[string]string{
				"main.tf": `variable "A" {
				description = "Testing!"
			}`,
			},
			Request: &proto.Provision_Request{
				Type: &proto.Provision_Request_Start{
					Start: &proto.Provision_Start{
						ParameterValues: []*proto.ParameterValue{{
							DestinationScheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							Name:              "A",
							Value:             "example",
						}},
					},
				},
			},
			Response: &proto.Provision_Response{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			},
		},
		{
			Name: "missing-variable",
			Files: map[string]string{
				"main.tf": `variable "A" {
			}`,
			},
			Response: &proto.Provision_Response{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Error: "exit status 1",
					},
				},
			},
		},
		{
			Name: "single-resource",
			Files: map[string]string{
				"main.tf": `resource "null_resource" "A" {}`,
			},
			Response: &proto.Provision_Response{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "A",
							Type: "null_resource",
						}},
					},
				},
			},
		},
		{
			Name: "bad-syntax-1",
			Files: map[string]string{
				"main.tf": `a`,
			},
			ErrorContains:     "configuration is invalid",
			ExpectLogContains: "Argument or block definition required",
		},
		{
			Name: "bad-syntax-2",
			Files: map[string]string{
				"main.tf": `;asdf;`,
			},
			ErrorContains:     "configuration is invalid",
			ExpectLogContains: `The ";" character is not valid.`,
		},
		{
			Name: "destroy-no-state",
			Files: map[string]string{
				"main.tf": `resource "null_resource" "A" {}`,
			},
			Request: &proto.Provision_Request{
				Type: &proto.Provision_Request_Start{
					Start: &proto.Provision_Start{
						State: nil,
						Metadata: &proto.Provision_Metadata{
							WorkspaceTransition: proto.WorkspaceTransition_DESTROY,
						},
					},
				},
			},
			ExpectLogContains: "nothing to do",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			for path, content := range testCase.Files {
				err := os.WriteFile(filepath.Join(directory, path), []byte(content), 0600)
				require.NoError(t, err)
			}

			request := &proto.Provision_Request{
				Type: &proto.Provision_Request_Start{
					Start: &proto.Provision_Start{
						Directory: directory,
					},
				},
			}
			if testCase.Request != nil {
				request.GetStart().ParameterValues = testCase.Request.GetStart().ParameterValues
				request.GetStart().State = testCase.Request.GetStart().State
				request.GetStart().DryRun = testCase.Request.GetStart().DryRun
				request.GetStart().Metadata = testCase.Request.GetStart().Metadata
			}
			if request.GetStart().Metadata == nil {
				request.GetStart().Metadata = &proto.Provision_Metadata{}
			}

			response, err := api.Provision(ctx)
			require.NoError(t, err)
			err = response.Send(request)
			require.NoError(t, err)

			gotExpectedLog := testCase.ExpectLogContains == ""
			for {
				msg, err := response.Recv()
				if msg != nil && msg.GetLog() != nil {
					if testCase.ExpectLogContains != "" && strings.Contains(msg.GetLog().Output, testCase.ExpectLogContains) {
						gotExpectedLog = true
					}

					t.Logf("log: [%s] %s", msg.GetLog().Level, msg.GetLog().Output)
					continue
				}
				if testCase.ErrorContains != "" {
					require.ErrorContains(t, err, testCase.ErrorContains)
					break
				}
				require.NoError(t, err)

				if msg.GetComplete() == nil {
					continue
				}

				require.NoError(t, err)

				// Remove randomly generated data.
				for _, resource := range msg.GetComplete().Resources {
					sort.Slice(resource.Agents, func(i, j int) bool {
						return resource.Agents[i].Name < resource.Agents[j].Name
					})

					for _, agent := range resource.Agents {
						agent.Id = ""
						if agent.GetToken() == "" {
							continue
						}
						agent.Auth = &proto.Agent_Token{}
					}
				}

				if testCase.Response != nil {
					resourcesGot, err := json.Marshal(msg.GetComplete().Resources)
					require.NoError(t, err)

					resourcesWant, err := json.Marshal(testCase.Response.GetComplete().Resources)
					require.NoError(t, err)

					require.Equal(t, testCase.Response.GetComplete().Error, msg.GetComplete().Error)

					require.Equal(t, string(resourcesWant), string(resourcesGot))
				}
				break
			}

			if !gotExpectedLog {
				t.Fatalf("expected log string %q but never saw it", testCase.ExpectLogContains)
			}
		})
	}
}

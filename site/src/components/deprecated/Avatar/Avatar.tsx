import { type Interpolation, type Theme, css, useTheme } from "@emotion/react";
import MuiAvatar, {
	type AvatarProps as MuiAvatarProps,
	// biome-ignore lint/nursery/noRestrictedImports: Used as base component
} from "@mui/material/Avatar";
import { visuallyHidden } from "@mui/utils";
import { type FC, useId } from "react";
import { getExternalImageStylesFromUrl } from "theme/externalImages";

export type AvatarProps = MuiAvatarProps & {
	size?: "xs" | "sm" | "md" | "xl";
	background?: boolean;
	fitImage?: boolean;
};

const sizeStyles = {
	xs: {
		width: 16,
		height: 16,
		fontSize: 8,
		fontWeight: 700,
	},
	sm: {
		width: 24,
		height: 24,
		fontSize: 12,
		fontWeight: 600,
	},
	md: {},
	xl: {
		width: 48,
		height: 48,
		fontSize: 24,
	},
} satisfies Record<string, Interpolation<Theme>>;

const fitImageStyles = css`
  & .MuiAvatar-img {
    object-fit: contain;
  }
`;

/**
 * @deprecated Use `Avatar` from `@components/Avatar` instead.
 */
export const Avatar: FC<AvatarProps> = ({
	size = "md",
	fitImage,
	children,
	background,
	...muiProps
}) => {
	const fromName = !muiProps.src && typeof children === "string";

	return (
		<MuiAvatar
			{...muiProps}
			css={[
				sizeStyles[size],
				fitImage && fitImageStyles,
				(theme) => ({
					background:
						background || fromName ? theme.palette.divider : undefined,
					color: theme.palette.text.primary,
				}),
			]}
		>
			{typeof children === "string" ? firstLetter(children) : children}
		</MuiAvatar>
	);
};

/**
 * @deprecated Use `Avatar` from `@components/Avatar` instead.
 */
export const ExternalAvatar: FC<AvatarProps> = (props) => {
	const theme = useTheme();

	return (
		<Avatar
			css={getExternalImageStylesFromUrl(theme.externalImages, props.src)}
			{...props}
		/>
	);
};

type AvatarIconProps = {
	src: string;
	alt: string;
};

/**
 * Use it to make an img element behaves like a MaterialUI Icon component
 *
 * @deprecated Use `AvatarIcon` from `@components/Avatar` instead.
 */
export const AvatarIcon: FC<AvatarIconProps> = ({ src, alt }) => {
	const hookId = useId();
	const avatarId = `${hookId}-avatar`;

	// We use a `visuallyHidden` element instead of setting `alt` to avoid
	// splatting the text out on the screen if the image fails to load.
	return (
		<>
			<img
				src={src}
				alt=""
				css={{ maxWidth: "50%" }}
				aria-labelledby={avatarId}
			/>
			<div id={avatarId} css={{ ...visuallyHidden }}>
				{alt}
			</div>
		</>
	);
};

const firstLetter = (str: string): string => {
	if (str.length > 0) {
		return str[0].toLocaleUpperCase();
	}

	return "";
};

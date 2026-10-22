package views

import "context"

type environmentBannerKey struct{}

// WithEnvironmentBanner returns ctx carrying the notice a non-production instance shows
// at the top of every page (appenv.Env.Banner); empty in production. Layout reads it
// from the context it renders with.
func WithEnvironmentBanner(ctx context.Context, banner string) context.Context {
	return context.WithValue(ctx, environmentBannerKey{}, banner)
}

func environmentBanner(ctx context.Context) string {
	banner, _ := ctx.Value(environmentBannerKey{}).(string)
	return banner
}

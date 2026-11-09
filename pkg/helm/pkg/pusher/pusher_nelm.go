package pusher

// WithExtraTags sets additional tags pointing to the pushed chart manifest.
func WithExtraTags(tags []string) Option {
	return func(opts *options) {
		opts.extraTags = tags
	}
}

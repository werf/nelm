package action

// WithPushExtraTags sets additional tags pointing to the pushed chart, besides its version tag.
func WithPushExtraTags(tags []string) PushOpt {
	return func(p *Push) {
		p.extraTags = tags
	}
}

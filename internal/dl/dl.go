package dl

type Option func(*config)

func OptUserAgent(ua string) Option {
	return func(c *config) {
		if ua == "" {
			c.UserAgent = DefaultUA
		}
		c.UserAgent = ua
	}
}

type config struct {
	UserAgent string
}

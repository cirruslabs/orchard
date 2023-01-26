package responder

type Option func(responder Responder)

func WithHeaders(headers map[string]string) Option {
	return func(responder Responder) {
		for key, value := range headers {
			responder.SetHeader(key, value)
		}
	}
}

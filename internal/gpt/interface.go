package gpt

type GPT interface {
	Request(prompt, query string) (string, error)
}

package gpt

type GPT interface {
	QueryToCommand(query string) (string, error)
}

package replies

import "strings"

const (
	NoCommands = "Команды не нужны. Напиши трату, например: «кофе 350»."
	Failed     = "Не получилось обработать сообщение. Попробуй ещё раз."
	intro      = "Я помогу записывать расходы."
)

func Greeting(firstName string) string {
	hello := "Привет!"
	if name := strings.TrimSpace(firstName); name != "" {
		hello = "Привет, " + name + "!"
	}
	return hello + "\n\n" + intro
}

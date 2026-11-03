# Учёт семейных расходов

Члены семьи записывают траты через Telegram обычным текстом, например «кофе 350». Сервис сам определяет сумму, категорию и дату. Глава семьи смотрит расходы всех и может их править в веб-панели.

## Запуск

Нужен установленный Docker. Настройки лежат в `backend/.env`: база, доступ к LLM и токен Telegram. Этот файл в репозиторий не входит, его нужно создать из шаблона и вписать `LLM_API_KEY` и `TELEGRAM_BOT_TOKEN`.

```bash
cp backend/.env.example backend/.env
docker compose up --build -d
```

LLM настраивается переменными `LLM_API_KEY`, `LLM_MODEL` и `LLM_BASE_URL`. Подойдёт любой сервис с OpenAI-совместимым API chat completions и поддержкой tool calling. В шаблоне указан Groq с моделью `openai/gpt-oss-20b`.

После запуска панель главы семьи открывается на [http://localhost](http://localhost). API доступен на [http://localhost:8080](http://localhost:8080).

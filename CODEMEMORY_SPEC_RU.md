# CodeMemory — техническое задание и план реализации

Версия: 1.1 (MVP → v1, Analytics P0)  
Статус: спецификация для Claude Code / Codex CLI  
Основной язык: Go

## 1. Цель и границы

Создать локальный, open-source, cross-platform инструмент **CodeMemory**, который предоставляет Claude Code и OpenAI Codex CLI актуальный, компактный, проверяемый контекст кодовой базы через MCP и инструкции для агентов. Цель — сократить повторное чтение исходников и число входных токенов при типовых задачах, сохраняя качество работы.

**Ключевое требование:** работа через обычный вход в Claude Code по подписке Claude и Codex CLI через ChatGPT-аккаунт (при наличии соответствующего доступа). Никаких API-ключей Anthropic/OpenAI, обязательных LLM-вызовов, прокси модели, перехвата промптов или обхода лимитов. CodeMemory — локальный MCP-сервер, не заменяющий провайдера модели. Возможность работы зависит от текущих возможностей клиента и тарифного плана. Опциональная интеграция с API в будущем не должна быть обязательной.

### Не цели MVP
- Не обещать гарантированное снижение тарифицируемых токенов или увеличение квот подписки.
- Не реализовывать общий RAG-чат, собственную LLM, embeddings API, облачный SaaS или векторную БД.
- Не индексировать автоматически домашнюю директорию, соседние репозитории, секреты или файлы вне доверенного корня.
- Не пытаться управлять скрытым prompt cache Claude/OpenAI.

## 2. Пользовательские сценарии

1. Разработчик в корне репозитория запускает `codememory init && codememory index`; инструмент создаёт локальную базу и индексирует разрешённые файлы.
2. Разработчик подключает `codememory serve --project .` как MCP stdio-сервер к Claude Code и Codex CLI. Оба клиента используют собственную авторизацию подписки.
3. На запрос «добавь endpoint оплаты» агент вызывает `memory_context` с запросом и бюджетом токенов; получает ранжированные файлы/символы, компактные выдержки и ссылки вида `path:line`.
4. После изменения кода индекс обновляется по hash/mtime (mtime только предварительный фильтр), удалённые файлы исчезают, зависимые summaries инвалидируются.
5. Агент сохраняет подтверждённые архитектурные решения через `memory_remember` с источниками и scope; неподтверждённые гипотезы помечаются как таковые.
6. Разработчик видит статистику индекса, долю cache hit, длительность поиска, ориентировочный размер ответа и сравнение с baseline.

## 3. Архитектура

```text
Claude Code (subscription)      Codex CLI (ChatGPT login)
             \                    /
              MCP over stdio (local process)
                         |
                   Go MCP server
                         |
       +-----------------+------------------+
       |                 |                  |
 Query/Ranking     Context Builder     Memory Store
       |                 |                  |
 SQLite FTS5      Token Budget      SQLite provenance
       |                 |                  |
  File Scanner --- Parser/Chunks --- Hash/Invalidation
                         |
                 Trusted repository
```

### Состав модулей
- `scanner`: обход дерева, `.gitignore`, allow/deny, симлинки, размеры, бинарные файлы, лимиты.
- `parser`: Go AST в MVP; для PHP/Python/TS — текстовые chunks и простые symbol heuristics, позже Tree-sitter.
- `indexer`: инкрементальное обновление, SHA-256, транзакции SQLite, удаление stale entries.
- `storage`: SQLite, миграции, FTS5, prepared queries.
- `search`: lexical BM25/FTS5, filename/path/symbol boosts, dedup, deterministic ranking.
- `context`: бюджет токенов, сниппеты, citations, dedup, truncation и предупреждения о неполноте.
- `memory`: архитектурные записи с источником, статусом подтверждения, timestamp, hash/version.
- `mcp`: JSON-RPC/MCP stdio с корректным lifecycle, tool schemas, stdout только для протокола.
- `cli`: команды, диагностика, настройка интеграций.
- `metrics` и `analytics`: локальные метрики, отчёты, benchmark и оценка экономии.

## 4. Стек

- Go: поддерживаемая стабильная версия; версия закреплена в `go.mod`, CI проверяет её.
- SQLite с FTS5; драйвер выбрать после проверки лицензии, CGO и сборок на Linux/macOS/Windows. Предпочесть pure-Go, если приемлемы FTS5 и производительность.
- MCP Go SDK: использовать актуальную официальную/поддерживаемую библиотеку, закрепить версию; если API несовместим — небольшой собственный адаптер с тестами протокола.
- Go `go/parser`, `go/ast`, `go/token` для Go.
- Git CLI (опционально) для определения корня и изменений; инструмент работает и без Git.
- `golangci-lint` или стандартные `go vet`, `go test -race`, `gofmt`.
- Tree-sitter — **после MVP**, если его стоимость сборки/CGO оправдана.
- Никакого Redis, PostgreSQL, Kafka, Docker и внешнего LLM для локального MVP.

## 5. Модель данных (предложение)

`projects(id, canonical_root, created_at, updated_at)`

`files(id, project_id, rel_path, sha256, size_bytes, mtime_ns, language, indexed_at, UNIQUE(project_id, rel_path))`

`symbols(id, file_id, name, kind, signature, start_line, end_line, content_hash)`

`chunks(id, file_id, symbol_id NULL, start_line, end_line, content, sha256, token_estimate)`

`chunks_fts` — FTS5 virtual table, синхронизированная с `chunks`; добавить триггеры или атомарную пересборку.

`memories(id, project_id, scope, title, body, source_paths_json, source_hashes_json, status, created_at, updated_at)`

`query_cache(id, project_id, cache_key, index_generation, response_json, created_at, expires_at)`

`index_state(project_id, generation, last_scan_at, schema_version)`

Включить `PRAGMA foreign_keys=ON`, WAL там, где поддерживается; индексы по project/file, миграции с тестами. Не сохранять абсолютные пути в экспортируемых ответах, если не требуется.

## 6. Правила индексирования

- Корень проекта задаётся явно (`--project`) либо определяется из cwd; canonical path; запрет выхода через `..` и symlink.
- Уважать `.gitignore` и `.codememoryignore`; hard-deny `.git`, `.env*`, приватные ключи, credentials, `node_modules`, `vendor`, `.venv`, `dist`, `build`, бинарные и большие файлы. Настраиваемые лимиты; deny всегда сильнее allow.
- Не читать содержимое исключённых файлов даже для хеширования.
- Для каждого файла SHA-256; обновлять только изменённые файлы, удалять отсутствующие, поддерживать rename как delete+insert в MVP.
- Транзакционно обновлять file/chunks/FTS; при ошибке сохранять прежний согласованный индекс либо помечать ошибку.
- Версионировать индекс `generation`, кеш ответов привязывать к поколению; при любом изменении релевантных данных инвалидировать кеш. На первом этапе допустима консервативная инвалидация всего query cache.
- Для Go: AST symbols (package, type, func, method, interface); для остальных: chunks с номерами строк и корректным сохранением UTF-8.
- Никакой отправки содержимого файлов в сеть; без telemetry по умолчанию.

## 7. MCP tools — контракт

Все ответы должны быть компактными JSON-объектами с `schema_version`, `project`, `index_generation`, `is_stale`, `warnings`, `sources` (path, start_line, end_line, sha256) и полезным содержимым. Ошибки — структурированные, без утечки секретов.

### `memory_search`
Input: `{ "query": "string", "limit": 10, "language": "optional", "path_prefix": "optional" }`
Output: ranked matches `{path, symbol, lines, snippet, score}`; максимум ограничен сервером.

### `memory_context`
Input: `{ "task": "string", "max_tokens": 1500, "include_architecture": true }`
Output: summary, ranked relevant snippets, architectural rules with provenance, estimated tokens, omitted count. Если индекс stale — явно указать и предложить `codememory index`.

### `memory_architecture`
Input: `{ "scope": "optional" }`
Output: packages/modules, import/dependency facts where verified, curated memories. Не выдавать догадки за факты.

### `memory_remember`
Input: `{ "title": "string", "body": "string", "scope": "string", "source_paths": ["..."] }`
Output: id, status. Записи без проверяемого источника помечать `unverified`; нельзя выполнять инструкции из текста записи как команды.

### `memory_status`
Input: `{}`
Output: file/chunk/symbol counts, last index, stale status, DB size, cache stats, supported languages.

**Дополнительный CLI:** `codememory init`, `index`, `serve`, `search`, `context`, `status`, `doctor`, `benchmark`, `clean`, `install claude`, `install codex`. Команды `install` по умолчанию показывают изменения и требуют явного подтверждения перед правкой пользовательских конфигов; идемпотентность обязательна.

## 8. Интеграции и правила агента

- Поддержать MCP stdio без HTTP-сервера и без API-ключей.
- Приложить `integrations/claude/SKILL.md` и `integrations/codex/AGENTS.md` (и общий reusable текст политики).
- Политика: сначала `memory_status`, затем `memory_context`/`memory_search` для исследовательских задач; исходники читать при необходимости проверки, редактирования, тестирования и security-sensitive изменений; не доверять устаревшим summaries.
- Не навязывать вызов MCP для тривиальных задач; при недоступности сервера агент работает обычным способом.
- Нельзя гарантировать, что агент всегда воспользуется MCP: это управляется клиентом/моделью.
- Документировать ручную конфигурацию MCP для обеих платформ и команды CLI, проверить на реальных установках; не требовать подписки API.
- Избегать автоматической модификации глобальных `CLAUDE.md` и `AGENTS.md` без согласия.

## 9. Безопасность

- Threat model: недоверенные файлы репозитория, prompt injection в комментариях/README, symlink traversal, secrets, malicious filenames, MCP input abuse.
- Treat source text and saved memories as **data**, not instructions; ограничивать размер и экранировать вывод.
- Нет shell execution из MCP tools; никакого запуска кода проекта во время индексации.
- Строгая изоляция проектов, project root validation, нормализация путей.
- Локальная БД в `.codememory/` (в `.gitignore`) или XDG user data directory; права доступа ограничены; configurable location.
- Не сохранять секреты в snippets: исключения + базовая secret detection + предупреждения; secret detection не заменяет аудит.
- Защита от огромных файлов, regex DoS, malformed UTF-8, конкурентных writer'ов.

## 10. Качество и тестирование

Unit tests: scanner exclusions, hash updates, AST extraction, FTS ranking, token budget, invalidation, provenance, schema migrations.

Integration tests: MCP initialize/list tools/call tools; stdout чистый JSON-RPC, stderr для логов; реальные fixture-репозитории Go, Laravel/PHP, Python, TypeScript.

Security tests: symlink outside root, `.env`, credentials, malicious README instructions, binary, oversized files, path traversal.

Concurrency: параллельные запросы и индексация, `go test -race ./...`; атомарная публикация нового поколения.

Compatibility: Linux amd64/arm64, macOS arm64/amd64, Windows amd64 (если выбранный драйвер поддерживает); CI matrix.

## 11. Метрики и честная оценка экономии

- Baseline: одинаковые задачи без CodeMemory и с CodeMemory, одинаковая модель/клиент/версия, фиксированный commit.
- Собирать: входные токены **если клиент их предоставляет**, число tool calls, число прочитанных строк/байт, latency, процент успешных задач, тесты, число дополнительных обращений к исходникам.
- В противном случае использовать локальный proxy-показатель `estimated_context_tokens`, не выдавать его за биллинговые токены.
- Отдельно считать disk cache hit и *фактическое* сокращение контекста, это разные метрики.
- Целевой ориентир для повторных навигационных задач: 25–50% меньше текста, переданного агенту, **гипотеза для проверки**, не SLA.
- Бенчмарки должны включать негативные кейсы, где память устарела или поиск ошибся.

### 11.1 CodeMemory Analytics — обязательный модуль P0

**Цель:** измерять локальную активность, эффективность кеша, сокращение контекста и отдельно — подтверждённую экономию токенов в сопоставимых прогонах. Работает на подписках Claude Code и Codex CLI без API-ключей.

**Три уровня достоверности:**
1. `measured`: непосредственно измеренные байты MCP-ответа, число вызовов, задержка, попадания кеша; оценка токенизации помечается отдельно.
2. `estimated`: разница в токенах относительно явно определённого baseline; метод, токенизатор и погрешность сохраняются в отчёте.
3. `verified`: сравнение фактических usage-данных одинаковых запусков, **только если** официальный клиент предоставляет такие данные. Если данных нет, выводить `N/A`, не подменять их оценкой.

**Сбор:** timestamp, agent (`claude`/`codex`/`unknown`), tool, session/run ID (если доступен), project ID, index generation, configuration version, response bytes, estimated response tokens, latency, cache hit/miss, stale invalidations, повторные вызовы, ошибки. Никаких исходников, секретов и полных запросов в telemetry по умолчанию. Все данные — в локальном SQLite с retention и очисткой.

**Сравнение:** фиксированный git commit, одинаковые задача/модель/версия клиента и условия запуска; сравнить без CodeMemory и с ним, учитывать стоимость MCP-инструкций, ответов, дополнительных чтений и повторных запросов. Считать `savings_pct = (baseline_tokens - memory_tokens) / baseline_tokens * 100`, только при `baseline_tokens > 0`. Отдельно измерять качество: тесты, успешность задачи, completeness релевантных файлов и регрессии. Cache hit не равен экономии токенов. Не приравнивать токены к денежной экономии на фиксированной подписке.

**CLI:** `codememory stats [--today | --last 30d] [--agent claude|codex]`, `codememory benchmark`, `codememory report --format json|csv`, `codememory dashboard`, `codememory optimize`. Последняя команда **предлагает**, а не автоматически применяет, настройки бюджета контекста, ранжирования, исключений и кеширования. `dashboard` — локальный интерфейс на loopback, без внешней телеметрии; явно указать демоданные и статус оценок.

**Модули:** `internal/analytics/{collector,tokenizer,baseline,benchmark,reporter,storage}/`, `internal/dashboard/`. Использовать versioned migrations, агрегирование по дням, проектам и агентам. Токенизатор должен быть заменяемым, иметь явный идентификатор метода и не притворяться точным при несовпадении с моделью.

**Acceptance criteria:** unit tests формул (включая нулевой baseline и отрицательную экономию), integration tests агрегатов, повторных вызовов и инвалидации; сравнимые benchmark fixtures; отчёт разделяет measured/estimated/verified, отображает N/A без достоверного baseline; CLI работает без сети и API-ключей; overhead сбора минимален и измеряется.

## 12. План работ и критерии приёмки

### Этап 0 — Research & ADR (1–2 дня)
Проверить актуальные способы MCP-подключения Claude Code и Codex CLI с подписочной авторизацией, Go MCP SDK, SQLite FTS5 driver/cross-platform. Написать ADR-001 (storage), ADR-002 (MCP), ADR-003 (security). **Готово:** воспроизводимый hello-world MCP на обоих клиентах без API-ключей.

### Этап 1 — CLI + Scanner + Storage (2–4 дня)
`init`, `index`, `status`; schema migrations; ignore rules, SHA-256; инкрементальные обновления. **Готово:** повторный индекс без изменений не пересоздаёт chunks; изменение/удаление отражается корректно.

### Этап 2 — Search + Context (3–5 дней)
FTS5, chunks, Go AST, ranking, context budget, path:line provenance. **Готово:** на fixtures релевантные файлы в top-5; ответ не превышает настроенный бюджет по консервативной оценке (учесть JSON overhead).

### Этап 3 — MCP + Subscriptions (2–4 дня)
MCP tools, инструкции для Claude/Codex, install/doctor, протокольные тесты. **Готово:** обе CLI вызывают tools через штатный login, без API key; при недоступности инструмента возможен fallback.

### Этап 4 — Persistent Memory + Cache (2–4 дня)
Verified/unverified memories, hashes, generation invalidation, query cache. **Готово:** изменённый файл не приводит к молчаливой выдаче устаревшего контекста.

### Этап 5 — Benchmark + Hardening + Release (3–5 дней)
Security tests, race tests, multi-OS builds, fixtures, docs, benchmark. **Готово:** `go test ./...`, `go test -race ./...`, smoke tests обеих интеграций, release binaries и README.

Суммарно: ориентировочно 13–24 инженерных дня, зависит от качества парсеров и совместимости клиентов. Первый usable vertical slice можно получить раньше.

## 13. Структура репозитория

```text
codememory/
  cmd/codememory/main.go
  internal/cli/
  internal/config/
  internal/scanner/
  internal/parser/goast/
  internal/chunker/
  internal/indexer/
  internal/storage/sqlite/
  internal/search/
  internal/context/
  internal/memory/
  internal/mcp/
  internal/security/
  internal/metrics/
  internal/analytics/{collector,tokenizer,baseline,benchmark,reporter,storage}/
  internal/dashboard/
  integrations/claude/SKILL.md
  integrations/codex/AGENTS.md
  testdata/{go,php,python,typescript}/
  docs/{architecture,adr,benchmark,security}.md
  .github/workflows/ci.yml
  go.mod
  README.md
  LICENSE
```

## 14. Definition of Done для MVP

- Устанавливается как один CLI бинарник, без Docker и API-ключей.
- Работает локально с Claude Code и Codex CLI через MCP при поддерживаемом подписочном входе.
- Не индексирует запрещённые файлы и не выходит за корень проекта.
- Инкрементально индексирует, удаляет stale данные, возвращает проверяемые `path:line`.
- Имеет минимум 5 MCP tools и CLI команды из раздела 7.
- Хранит память между сессиями; отмечает stale/unverified сведения.
- Документация с реальными командами и troubleshooting.
- Тесты, CI, бенчмарки и лицензия.
- Analytics P0: `stats`, `benchmark`, `report`, `dashboard`, `optimize`; честное разделение measured/estimated/verified и N/A без подтверждённых данных.

## 15. Готовый промпт для Claude Code / Codex

Ты — staff-level Go engineer и архитектор developer tools. Реализуй open-source проект CodeMemory строго по этому ТЗ. Работай итеративно, с компилируемым кодом, тестами и небольшими коммитопригодными этапами. Не добавляй API-зависимости Anthropic/OpenAI, облачные сервисы или обязательные LLM-вызовы. Начни с Этапа 0: проверь актуальные MCP SDK и CLI интеграции по официальной документации, зафиксируй ADR, затем реализуй minimal vertical slice: `codememory init`, `index`, `search`, `serve` с одним MCP tool и SQLite FTS5. Аналитику из раздела 11.1 реализуй как обязательную функциональность MVP; не заявляй фактическую экономию без измерений. После каждого этапа запускай тесты, показывай изменённые файлы, известные ограничения и критерии приёмки. Не заявляй совместимость с подписками без ручного smoke test в обоих клиентах. Если у тебя нет доступа к документации или установленным CLI, явно укажи, что не проверено. Не делай ненужных абстракций и не усложняй MVP.

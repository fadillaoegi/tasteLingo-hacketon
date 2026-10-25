# TasteLingo

An AI agent that builds an English-learning plan from the films, TV shows, books, podcasts and
music you already love, using the [Qloo](https://www.qloo.com/) taste graph. Built for the
Qloo Agentic Hackathon (2026).

> **Status:** early development. Live Qloo and Gemini are enabled and verified in the local
> configuration. The example configuration defaults to mock mode until you supply your own keys.
> Mock data is synthetic, every mock title ends in “(mock)”, and the UI shows a development-mode
> banner when a mock provider is active.

## Background

Language learners improve fastest with lots of *comprehensible input*: watching, reading and
listening to content they understand and enjoy. Courses and learning apps, however, give everyone
the same material. A learner who loves anime, true-crime podcasts or K-dramas gets the same reading
as everyone else, motivation drops, and many people quit. Asking a general chatbot to “recommend
English films” returns a generic list of popular titles that may not fit the learner's taste or level.

TasteLingo uses Qloo's taste graph to find English-language content that matches what a learner
actually likes, then an agent turns that content into a daily plan and exercises at the learner's
level. We do **not** claim proven effects on motivation or learning speed; the demo shows what can
be observed: recommendation relevance and the exercises generated.

## How it works

1. **Onboarding** — pick 1–5 favorites (searched via Qloo `/search`), a CEFR level (A2–C1), a goal
   (conversation, work, academic) and daily time (15/30/60 min).
2. **Planning agent** — a tool-calling loop in the Go backend. Tools:
   `search_entities`, `get_tags`, `get_taste_insights` (Qloo `/v2/insights` across movie, TV,
   book, podcast and artist) and `build_plan`.
3. **Grounding guard** — `build_plan` rejects any title that was not returned by a taste-graph tool
   in the same run, so the LLM cannot invent recommendations.
4. **Agent trace** — every reasoning step, tool call and result is stored and visible in the UI.

5. **Without Qloo vs with TasteLingo** — the same learner facts go to the LLM alone (no tools);
   the two plans are shown side by side, with LLM-only titles marked as unverified.
6. **Practice** — `generate_exercise` writes vocabulary, comprehension questions and a speaking or
   writing prompt from the item's description, tags and metadata. Answers are scored and the LLM gives
   short feedback. The answer key is kept out of the response and the visible trace.
7. **Adaptation** — feedback per day (love it, not my taste, too hard, too easy) lets `adapt_plan`
   replan: liked items become Qloo signals, disliked items are rejected, and the plan explains what changed.
8. **Demo learner** — one click creates Rina (B1, job interviews) so judges can try it without setup.

## Brand identity

The TasteLingo mark brings folded book pages and a conversation bubble into one sculpted 3D
symbol: knowledge becoming expression. The connected gold nodes represent personal interests
leading to new learning discoveries. Teal connects the identity to the app's existing palette;
mint highlights suggest growth, while warm gold marks moments of discovery.

The transparent master and optimized web exports live in `tastelingo_FE/public/brand/`.
The original generation prompt is saved alongside them in `generation-prompt.txt`.

The interface extends this identity with a CSS 3D taste studio: floating interest cards,
gentle pointer parallax, and an interactive explanation of taste → connections → learning.
The studio and preference preview reflect the learner's actual form choices. Motion can be
paused and follows the device's reduced-motion preference; no extra model calls are made for
animation. Loading stages reflect known request phases rather than simulated percentages.

## Stack

- **Backend:** Go, Clean Architecture (`domain`, `application` use cases + ports, `adapters`:
  Qloo client/mock, Gemini/mock LLM, PostgreSQL/memory repository, HTTP API)
- **Frontend:** React + TypeScript + Vite + Tailwind CSS (responsive)
- **Database:** PostgreSQL

## Run locally

Requirements: Go 1.26, Node 24 + pnpm, Docker.

```bash
cp .env.example .env            # keep real keys only here; .env is gitignored
docker compose up -d postgres   # PostgreSQL on 127.0.0.1:55442
cd tastelingo_BE && go run ./cmd/server          # API on 127.0.0.1:8110
cd tastelingo_FE && pnpm install && pnpm dev     # UI on http://127.0.0.1:5190
```

Modes (in `.env`): `QLOO_MODE=mock|live`, `LLM_MODE=mock|gemini`, `STORE_MODE=postgres|memory`.

The frontend uses the API at `http://127.0.0.1:8110` by default. To use a different local API,
set `TASTELINGO_API_PROXY=http://127.0.0.1:8111` in `tastelingo_FE/.env.local` (gitignored).
Vite reloads this setting automatically.

To enable Gemini, set `LLM_MODE=gemini` and `GEMINI_API_KEY` in your local `.env`, then restart
the API. `GEMINI_MODEL` selects the primary model (default `gemini-3.5-flash-lite`).
`GEMINI_FALLBACK_MODELS` is an ordered, comma-separated list; the default is
`gemini-3.1-flash-lite,gemini-3.8-flash,gemini-3.6-flash`.

`GEMINI_AUTO_DISCOVER=true` (default) also discovers compatible text models from the account's
Gemini model catalog when the configured models are unavailable. The catalog is cached for
15 minutes; image, audio, live and specialized models are excluded. To disable all fallback,
set `GEMINI_FALLBACK_MODELS=` and `GEMINI_AUTO_DISCOVER=false`.

On a request timeout, HTTP 429 (quota/rate limit), 404 (model unavailable), or 500/502/503/504, the adapter tries
the next available model once. Each model request has a 20-second timeout, within the overall
120-second planning budget. A failed model cools down for at least 60 seconds, honoring longer
`Retry-After` or Gemini `RetryInfo` hints. New conversations retry the primary after its cooldown;
ongoing tool conversations prefer their last healthy model. Transferred tool history follows
[Google's thought-signature guidance](https://ai.google.dev/gemini-api/docs/generate-content/thought-signatures#faqs).
Authentication and invalid-request errors stop immediately. If all models are unavailable, the
request returns an error; it never silently switches to a mock LLM. Fallback events are logged
without keys or prompts. Changing models cannot guarantee success when every model or the
whole project's quota is exhausted; [Google applies limits per project](https://ai.google.dev/gemini-api/docs/rate-limits).

The frontend restores a browser profile through `GET /api/learners/{id}/session`, which returns
a nullable learner/plan for expired or unfinished sessions. It does not request a missing plan
while one is being generated. Plan loading shows the current stage, elapsed time, cancellation,
and a retry using the saved profile. A previously completed demo is reused on subsequent demo
visits in the same browser when its provider modes still match.

Tests: `cd tastelingo_BE && go test -race ./...` and `cd tastelingo_FE && pnpm build`.

Frontend tests: `cd tastelingo_FE && pnpm test`. Optional visual/browser checks:
`pnpm test:ui`, using installed Chrome (`CHROME_PATH` overrides its location).
The browser runner checks desktop and mobile layouts, interactive stages, pause/reduced
motion, and a synthetic loading/cancel flow without creating a paid plan. Add
`--learner-id <existing-id>` to inspect a saved plan as well. Screenshots stay in `.local/motion-preview/`.

## Deploy

The root `Dockerfile` builds one image that serves the API and the built frontend from the same
origin (`STATIC_DIR`), listens on `$PORT`, and enables rate limiting behind a proxy.

```bash
docker build -t tastelingo .
docker run -p 8080:8080 --env-file .env -e API_ADDR= tastelingo
```

`render.yaml` is a [Render](https://render.com) Blueprint for the same image. Use a managed
PostgreSQL such as [Neon](https://neon.tech) for `DATABASE_URL` (with `sslmode=require`), and enter
`QLOO_API_KEY` and `GEMINI_API_KEY` as secrets in the hosting dashboard.

Rate limits (per hour): agent runs (plan, compare, adapt, exercise, answer feedback) default to 20
per client IP and 300 in total; search and other light writes default to 200 per IP and 3000 in total.
When a limit is reached the API returns HTTP 429 with `Retry-After`. Behind a proxy, the client IP
is the right-most `X-Forwarded-For` entry (`TRUST_PROXY=true`).

## Data and privacy

- API keys stay in the server environment, never in the browser or the repository.
- Qloo responses may be cached privately on the server; **they are never committed** to this
  repository. Test fixtures use synthetic data.
- Exercises are generated from entity descriptions, tags and metadata, not from copyrighted content.

## Limitations

- Qloo search, tags and recommendations across the five supported media types have been verified
  against live hackathon responses. Broader API features remain outside this version.
- No speech recognition, classroom mode, mobile app or payments in this version.

## License

[MIT](LICENSE)

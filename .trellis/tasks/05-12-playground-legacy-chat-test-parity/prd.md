# brainstorm: playground legacy chat test parity

## Goal

Restore and improve feature parity between the default frontend Playground and the legacy/classic chat testing Playground so users can use the new Playground as a practical debugging and testing surface, not only a minimal chat box.

## What I already know

* User reports the current Playground module does not compatibly cover the old chat test page and many features are missing.
* Current default frontend Playground lives under `web/default/src/features/playground/` and route `web/default/src/routes/_authenticated/playground/index.tsx`.
* Legacy/classic implementation lives under `web/classic/src/pages/Playground/`, `web/classic/src/components/playground/`, `web/classic/src/hooks/playground/`, and `web/classic/src/constants/playground.constants.js`.
* Current default Playground already supports basic model/group selection, streaming/non-streaming chat, image-generation model path, message copy/regenerate/edit/delete, local config/messages storage, reasoning display, generated-image markdown parsing, and stop generation.
* Current default input shows Attach/Search actions, but they currently only display “Feature in development” and do not attach images/files or influence request payload.
* Current default config only includes `model`, `group`, `temperature`, `top_p`, `max_tokens`, penalties, `seed`, and `stream`; parameter controls are not exposed in the visible page yet.
* Legacy/classic Playground includes a left settings panel, parameter toggles, image URL/input support, custom request body mode, preview/actual request/response debug panel, raw SSE viewer, config import/export/reset, session list/new session/switching, conversation_id persistence for ChatGPT Web/image flows, paste/upload image handling, and mobile floating buttons.
* Default frontend must follow `web/default/AGENTS.md`: React 19, TypeScript, Base UI/Tailwind patterns, i18n via `useTranslation()`, Bun for frontend checks, no new dependencies unless explicitly needed.

## Assumptions (temporary)

* “旧版的聊天测试页面” refers primarily to the classic Playground implementation in `web/classic/src/pages/Playground` and related components/hooks.
* The goal is to port/modernize behavior into `web/default` rather than revive or embed `web/classic` components directly.
* MVP should prefer feature parity for debugging/testing workflows over pixel-perfect visual parity with the Semi UI classic page.
* Existing `web/default` `ai-elements` chat surface should be preserved where possible.

## Open Questions

* No blocking questions remain; ready for implementation confirmation.

## Requirements (evolving)

* Preserve current default Playground basic chat and image-generation behavior.
* Add missing legacy capabilities in a way that matches the default frontend design system.
* MVP scope follows Approach A: settings/debug parity first.
* Include parameter controls, stream toggle, custom request body mode, preview request body, actual request body, response/debug output, raw SSE capture, and config import/export/reset.
* Use classic workbench layout: desktop left settings panel + center chat + right debug panel; mobile uses drawer/floating controls for settings/debug.
* Keep localStorage compatibility/migration in mind because both versions use `playground_config` and `playground_messages` keys.
* Do not modify protected project/organization branding.

## Acceptance Criteria (evolving)

* [ ] User can inspect preview request body before sending.
* [ ] User can inspect actual request and response after sending.
* [ ] Stream responses expose enough raw SSE/debug information for troubleshooting.
* [ ] User can configure model/group and supported request parameters from the Playground UI.
* [ ] User can use custom request body mode for advanced testing.
* [ ] Config import/export/reset is available for Playground settings and current messages.
* [ ] Session create/switch persistence is deferred from MVP unless needed by the chosen layout.
* [ ] Image attachment/reference behavior is explicitly deferred from MVP.
* [ ] Frontend typecheck and targeted lint pass for changed files.

## Definition of Done (team quality bar)

* Tests added/updated where practical for pure logic and payload builders.
* `cd web/default && bun run typecheck` passes.
* Targeted lint on changed frontend files passes; broader lint status documented if unrelated failures exist.
* i18n keys/translations updated for new UI text.
* Rollback is possible by reverting the Playground feature files only.

## Out of Scope (explicit)

* No backend API changes unless repo inspection shows a default-frontend parity feature cannot work with existing `/pg/*` endpoints.
* No direct reuse of Semi UI classic components in `web/default`.
* No new frontend dependencies unless a specific feature cannot be implemented with existing stack.
* Pixel-perfect clone of the old page is not assumed unless the user explicitly requires it.
* Full multi-session management, image URL/paste/upload reference handling, and ChatGPT Web/image `conversation_id` continuity are deferred from MVP.

## Technical Notes

* Current default route: `web/default/src/routes/_authenticated/playground/index.tsx` renders `Playground`.
* Current default files inspected: `web/default/src/features/playground/index.tsx`, `api.ts`, `constants.ts`, `types.ts`, `components/playground-input.tsx`, `components/playground-chat.tsx`, `hooks/use-playground-state.ts`, `hooks/use-chat-handler.ts`, `hooks/use-stream-request.ts`, `lib/payload-builder.ts`, `lib/storage.ts`.
* Legacy/classic files inspected: `web/classic/src/pages/Playground/index.jsx`, `web/classic/src/components/playground/{SettingsPanel,DebugPanel,CustomRequestEditor,ConfigManager,SSEViewer,CodeViewer,ImageUrlInput,FloatingButtons,ChatArea}.jsx`, `web/classic/src/hooks/playground/{usePlaygroundState,useApiRequest,useSyncMessageAndCustomBody,useMessageEdit,useMessageActions}.jsx`, `web/classic/src/components/playground/configStorage.js`, `web/classic/src/constants/playground.constants.js`.
* Legacy request-building features to map carefully: `buildApiPayload`, image generation payload, `conversation_id`/`fallback_prompt`, custom request body, debug data capture, raw SSE capture.
* Current default stream hook parses SSE but does not store debug request/response/SSE messages.
* Current default storage is simpler and lacks session storage/import/export; legacy storage includes `playground_sessions` and `playground_active_session_id` style concepts through `STORAGE_KEYS.SESSIONS` and `ACTIVE_SESSION_ID`.

## Expansion Sweep

### Future evolution

* Playground could become the primary admin/user diagnostic surface for routing, cache-hit, channel-affinity, and request-shape debugging.
* A future “request templates/presets” system could build on custom request body + preview/debug panels.

### Related scenarios

* Channel test dialog and Playground both exercise upstream relay paths; request preview/debug should remain consistent with relay payload semantics.
* ChatGPT Web / image-generation channels may need conversation continuity; session-level conversation ids need to be keyed by model/group/kind.

### Failure & edge cases

* Invalid custom JSON must not send malformed requests; it should show a clear validation state.
* Large pasted/base64 images can exceed localStorage; legacy code sanitizes or omits large data URLs.
* Switching sessions or stopping generation must close active SSE/polling requests safely.

## Candidate Approaches

### Approach A: MVP debug/settings parity first (Recommended)

* Add default-frontend settings/debug side panels, parameter controls, custom request body mode, preview/actual request/response capture, raw SSE capture, and config import/export/reset.
* Keep session management and image attachment/reference as second phase unless they are already easy to include.
* Pros: fastest path to fix the “chat testing” capability gap; lower risk and smaller diff.
* Cons: not full legacy parity; users who depend on multi-session/image paste may still see gaps.

### Approach B: Full legacy parity port in one task

* Port settings, debug, custom body, sessions, image attach/paste/reference, conversation id persistence, config import/export/reset, mobile floating controls in one coordinated implementation.
* Pros: closest to old page; fewer staged UX inconsistencies.
* Cons: broad frontend refactor, larger regression surface, harder to verify in one pass.

### Approach C: Compatibility shell with progressive feature modules

* First build a `PlaygroundWorkbench` shell with panels, state model, and debug contracts; then add legacy features behind small modules.
* Pros: clean long-term architecture and easier future extensions.
* Cons: more upfront structure; slower to deliver immediately visible parity.


## Decision (ADR-lite)

**Context**: The default Playground lacks several diagnostic features from the classic chat test page, but a full port would touch many state/storage/UI paths at once.

**Decision**: Use Approach A for this task: restore core settings/debug parity first. Use the classic workbench layout for desktop and drawer/floating controls for mobile.

**Consequences**: This delivers the most important testing/debugging capability quickly with lower regression risk. Multi-session management, image references, and conversation-id continuity remain known follow-up work unless later pulled into scope.

## Research References

* Inline repository comparison only; no external research needed at this stage because the reference implementation is in `web/classic`.

## Technical Approach

* Keep `web/default/src/features/playground/index.tsx` as the page coordinator, but split settings/debug UI into focused components under `components/`.
* Extend Playground state to include `showSettings`, `showDebugPanel`, `debugData`, `activeDebugTab`, `customRequestMode`, and `customRequestBody`.
* Extend payload building so preview, custom request body mode, and actual send use the same request shape.
* Extend stream/non-stream request hooks to report debug events: actual request, parsed/non-stream response, raw SSE messages, timestamps, and streaming state.
* Add config import/export/reset helpers in `lib/storage.ts` using existing localStorage keys, preserving current saved config/messages where possible.
* Use existing UI primitives in `web/default/src/components/ui/`; no new dependencies.

## Implementation Plan (small PR slices)

1. **State + payload contracts**
   * Extend Playground types/config/storage/debug state.
   * Add/adjust pure helpers for preview payload and custom request JSON handling.
   * Add focused tests if an existing frontend test runner path is practical; otherwise rely on TypeScript and targeted lint for this frontend-only module.
2. **Settings panel**
   * Add parameter controls, stream toggle, custom request body editor, and config import/export/reset UI.
   * Wire settings to existing `config` / `parameterEnabled` state.
3. **Debug panel + request instrumentation**
   * Add preview/request/response/SSE tabs.
   * Capture debug info for streaming and non-streaming requests.
4. **Workbench layout**
   * Desktop: left settings, center chat, right debug.
   * Mobile: floating buttons/drawer-style overlays for settings/debug.
5. **Verification + i18n**
   * Add/update i18n keys across supported locales.
   * Run `bun run typecheck` and targeted lint for changed files.

## Finalized MVP Scope

### Included

* Classic workbench layout.
* Settings panel for model/group, request parameters, stream toggle, custom request body mode.
* Preview request body.
* Actual request body and response debug views.
* Raw SSE capture/display for streaming responses.
* Config import/export/reset for Playground settings and current messages.

### Deferred

* Full multi-session create/switch persistence.
* Image URL/paste/upload reference workflow.
* ChatGPT Web/image `conversation_id` continuity.
* Pixel-perfect classic visual clone.

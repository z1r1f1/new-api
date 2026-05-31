# Internal Realtime Chat Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver in-repo one-to-one and group chat for authenticated `new-api` users, with persistent conversation/message storage, realtime delivery, presence, unread counts, and a React UI.

**Architecture:** Keep `new-api` as the source of truth for users, conversations, membership, messages, and permissions. Add a small embedded Centrifuge-based realtime layer to fan out validated events over WebSocket while Redis handles ephemeral presence and pub/sub. Keep the existing external chat preset feature intact and add the new internal messaging UI under a separate frontend namespace.

**Tech Stack:** Go 1.22, Gin, GORM, Redis, Centrifuge Go library, Gorilla WebSocket where needed, React 19, TanStack Router, TanStack Query, Zustand, Tailwind CSS, Bun, and `centrifuge-js`.

---

### Task 1: Chat schema and migrations

**Files:**
- Create: `model/chat_conversation.go`
- Create: `model/chat_conversation_member.go`
- Create: `model/chat_message.go`
- Create: `model/chat_read_state.go`
- Modify: `model/main.go` to register chat tables in the existing migration flow
- Test: `model/chat_conversation_test.go`, `model/chat_message_test.go`

- [ ] **Step 1: Write the failing model tests**

Add tests that assert the chat entities can be auto-migrated on SQLite and that the helper query methods can load:
- a direct conversation by the two member ids,
- a group conversation by id,
- messages by conversation id with stable ordering,
- read state by conversation/user pair.

- [ ] **Step 2: Run the targeted tests and confirm they fail**

Run: `go test ./model -run 'TestChat' -v`
Expected: fail because the chat models and helpers do not exist yet.

- [ ] **Step 3: Implement the minimal schema and query helpers**

Add portable GORM models and helper functions for:
- creating or finding a direct conversation,
- creating a group conversation,
- adding/removing members,
- inserting messages,
- updating read state,
- fetching conversation history.

- [ ] **Step 4: Run the model tests again**

Run: `go test ./model -run 'TestChat' -v`
Expected: PASS.

- [ ] **Step 5: Commit the schema slice**

```bash
git add model/chat_conversation.go model/chat_conversation_member.go model/chat_message.go model/chat_read_state.go model/main.go model/chat_conversation_test.go model/chat_message_test.go
git commit -m "feat: add chat persistence schema"
```

---

### Task 2: Chat service and realtime publishing

**Files:**
- Create: `service/chat/service.go`
- Create: `service/chat/conversation.go`
- Create: `service/chat/message.go`
- Create: `service/chat/membership.go`
- Create: `service/chat/realtime.go`
- Create: `service/chat/presence.go`
- Test: `service/chat/service_test.go`
- Test: `service/chat/realtime_test.go`

- [ ] **Step 1: Write the failing service tests**

Add tests that cover:
- rejecting a send from a non-member,
- creating a direct conversation once and reusing it on the second call,
- creating a group conversation with the creator as a member,
- incrementing unread counters for non-senders,
- publishing a realtime event only after the message is persisted.

- [ ] **Step 2: Run the targeted tests and confirm they fail**

Run: `go test ./service/chat -v`
Expected: fail because the package does not exist yet.

- [ ] **Step 3: Implement the chat service and realtime adapter**

Create a service package that:
- validates conversation membership and user status,
- writes messages through the model layer,
- updates last-message metadata and unread state,
- publishes events into the Centrifuge channel tree,
- stores and refreshes ephemeral presence keys in Redis.

- [ ] **Step 4: Run the service tests again**

Run: `go test ./service/chat -v`
Expected: PASS.

- [ ] **Step 5: Commit the backend logic slice**

```bash
git add service/chat/service.go service/chat/conversation.go service/chat/message.go service/chat/membership.go service/chat/realtime.go service/chat/presence.go service/chat/service_test.go service/chat/realtime_test.go
git commit -m "feat: add chat service and realtime fanout"
```

---

### Task 3: Chat API and authenticated realtime endpoint

**Files:**
- Create: `controller/chat.go`
- Create: `controller/chat_ws.go`
- Create: `router/chat-router.go`
- Modify: `router/api-router.go` to register the new router group
- Modify: `middleware/auth.go` only if a reusable chat auth helper is required
- Test: `controller/chat_test.go`, `router/chat-router_test.go`

- [ ] **Step 1: Write the failing controller and router tests**

Add tests that verify:
- unauthenticated requests are rejected,
- a member can create a conversation,
- a member can fetch message history,
- a non-member cannot post to the conversation,
- the WebSocket upgrade endpoint accepts an authenticated user and rejects a disabled user.

- [ ] **Step 2: Run the targeted tests and confirm they fail**

Run: `go test ./controller ./router -run 'TestChat' -v`
Expected: fail because the handlers and route registration do not exist yet.

- [ ] **Step 3: Implement the HTTP handlers and route registration**

Add API endpoints for:
- listing conversations,
- creating direct or group conversations,
- listing conversation history,
- sending messages,
- marking messages as read,
- managing group members,
- upgrading authenticated clients to the realtime connection.

- [ ] **Step 4: Run the controller and router tests again**

Run: `go test ./controller ./router -run 'TestChat' -v`
Expected: PASS.

- [ ] **Step 5: Commit the API slice**

```bash
git add controller/chat.go controller/chat_ws.go router/chat-router.go router/api-router.go controller/chat_test.go router/chat-router_test.go
git commit -m "feat: expose chat APIs and websocket"
```

---

### Task 4: Frontend internal chat feature and routes

**Files:**
- Create: `web/default/src/features/messaging/api.ts`
- Create: `web/default/src/features/messaging/types.ts`
- Create: `web/default/src/features/messaging/hooks/use-conversations.ts`
- Create: `web/default/src/features/messaging/hooks/use-messages.ts`
- Create: `web/default/src/features/messaging/components/conversation-list.tsx`
- Create: `web/default/src/features/messaging/components/message-thread.tsx`
- Create: `web/default/src/features/messaging/components/member-panel.tsx`
- Create: `web/default/src/routes/_authenticated/messages/index.tsx`
- Create: `web/default/src/routes/_authenticated/messages/$conversationId.tsx`
- Modify: `web/default/src/features/system-settings/maintenance/config.ts` or `web/default/src/hooks/use-sidebar-data.ts` only if a new sidebar entry is required
- Modify: `web/default/src/i18n/static-keys.ts`
- Modify: `web/default/src/i18n/locales/en.json`, `zh.json`, `fr.json`, `ru.json`, `ja.json`, `vi.json`
- Test: `web/default/src/features/messaging/*.test.ts`, route tests if needed

- [ ] **Step 1: Write the failing frontend tests**

Add tests that cover:
- rendering an empty conversation list,
- opening a conversation and rendering messages in order,
- disabling the send box when the current user is not a member,
- updating unread state after a message arrives through the realtime client.

- [ ] **Step 2: Run the targeted frontend tests and confirm they fail**

Run: `cd web/default && bun test src/features/messaging`
Expected: fail because the new feature does not exist yet.

- [ ] **Step 3: Implement the messaging feature and routes**

Build the new authenticated messaging area, wire it to the backend APIs, and connect a Centrifuge client hook that subscribes to the active user and conversation channels.

- [ ] **Step 4: Run typecheck, lint, and build**

Run:
- `cd web/default && bun run typecheck`
- `cd web/default && bun run lint`
- `cd web/default && bun run build`
Expected: PASS.

- [ ] **Step 5: Commit the frontend slice**

```bash
git add web/default/src/features/messaging web/default/src/routes/_authenticated/messages web/default/src/i18n/static-keys.ts web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/vi.json web/default/src/features/system-settings/maintenance/config.ts web/default/src/hooks/use-sidebar-data.ts
git commit -m "feat: add internal chat frontend"
```

---

### Task 5: Cross-layer verification and cleanup

**Files:**
- Modify: any chat-related files that need polish after tests
- Test: backend and frontend test suites touched by the new feature

- [ ] **Step 1: Run the focused backend checks**

Run:
- `go test ./model ./service/chat ./controller -run 'TestChat' -v`
- `go test ./router -run 'TestChat' -v`
Expected: PASS.

- [ ] **Step 2: Run the focused frontend checks**

Run:
- `cd web/default && bun run typecheck`
- `cd web/default && bun run lint`
- `cd web/default && bun run build`
Expected: PASS.

- [ ] **Step 3: Run a broader regression pass**

Run:
- `go test ./...`
- `cd web/default && bun test src/features/messaging`
Expected: PASS or a documented, explained skip if the repo does not expose a stable frontend test target beyond the focused messaging suite.

- [ ] **Step 4: Commit the final cleanup**

```bash
git add .
git commit -m "feat: complete internal realtime chat integration"
```

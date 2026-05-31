# Internal Realtime Chat Integration Design

> Goal: integrate lightweight one-to-one and group chat into `new-api` as a first-class in-repo feature, using Centrifuge as the realtime transport layer while keeping message ownership, permissions, and persistence inside `new-api`.

**Goal:** Add a self-hosted internal chat system to `new-api` that supports private conversations, group conversations, online presence, typing indicators, unread counts, and read receipts without introducing a separate chat service.

**Architecture:** `new-api` will own all chat business data and authorization. A small embedded realtime layer built on the Centrifuge Go library will fan out message and presence events over WebSocket, while PostgreSQL/MySQL/SQLite-compatible GORM tables persist the canonical chat state. The frontend will add a new internal chat area under the authenticated web app and connect to the backend through the same user session model used elsewhere in the project.

**Tech Stack:** Go 1.22, Gin, GORM, Redis, Centrifuge Go library, Gorilla WebSocket where needed for transport upgrades, React 19, TanStack Router, TanStack Query, Zustand, Tailwind CSS, Bun, and `centrifuge-js` on the web client.

---

## 1. Decision Summary

### Chosen approach
Use the Centrifuge Go library as an embedded realtime core inside `new-api`, not as a separate deployed IM server.

### Why this fits best
- It is lightweight enough to live inside the existing Go monolith.
- It provides the realtime primitives needed for message fanout, presence, recovery, and channel subscriptions.
- It matches the current Go/Redis stack better than a full IM platform.
- It avoids creating a second runtime, second deployment, and second authentication model.

### Rejected alternatives
- **Tinode**: complete IM platform, but too large and opinionated for direct embedding into `new-api`.
- **OpenIM**: powerful developer-facing IM platform, but it is also closer to a standalone IM ecosystem than a lightweight in-repo module.
- **Raw WebSocket only**: simpler initially, but would require recreating presence, recovery, channel authorization, and fanout patterns that Centrifuge already solves.

## 2. Scope

### In scope for the first release
- One-to-one direct messages.
- Group conversations.
- Message history and pagination.
- Read receipts.
- Unread counters.
- Presence and typing indicators.
- Conversation membership management.
- Frontend conversation list, message pane, and group members panel.

### Out of scope for the first release
- End-to-end encryption.
- Audio/video calls.
- File attachments and media storage.
- Message reactions.
- Message search across the full corpus.
- Cross-instance federation.
- Push notifications to mobile devices.

## 3. Backend Architecture

### 3.1 Layering
The feature follows the existing `router -> controller -> service -> model` pattern.

- **`router/`** registers HTTP and WebSocket endpoints.
- **`controller/`** handles request parsing, authentication, and HTTP response mapping.
- **`service/`** owns chat business rules, membership checks, unread counters, and event emission.
- **`model/`** owns persistence, schema migration, and database queries.

### 3.2 Data model
The backend will persist chat state in GORM tables compatible with SQLite, MySQL, and PostgreSQL.

Planned entities:
- `ChatConversation`
  - conversation identity
  - conversation type: direct or group
  - title / display name
  - owner / creator
  - timestamps
- `ChatConversationMember`
  - conversation id
  - user id
  - role in conversation
  - joined / left timestamps
  - mute / block / archive flags if needed later
- `ChatMessage`
  - conversation id
  - sender id
  - message type
  - plain text body for the first release
  - client message id for idempotency
  - delivery / edit / delete timestamps as needed
- `ChatReadState`
  - conversation id
  - user id
  - last read message id or sequence
  - unread counter support

Presence and typing will be transient realtime state backed by Redis TTL entries and realtime events, not permanent database rows.

### 3.3 Realtime transport
Centrifuge will handle:
- connection management,
- channel subscriptions,
- publish/fanout,
- presence,
- history / recovery support,
- reconnect recovery.

Channel design:
- `user:{userId}` for personal notifications and direct-message delivery hints.
- `conversation:{conversationId}` for conversation-scoped message fanout.
- `presence:{conversationId}` only if a separate ephemeral presence stream is useful; otherwise presence can be derived from the conversation channel subscription state.

The backend will be the source of truth for authorization. The realtime layer will only publish events after service-layer validation succeeds.

### 3.4 Authentication and authorization
The chat feature will reuse the existing `new-api` identity system.

Rules:
- Browser users must already be authenticated via the existing session-based dashboard flow.
- Chat endpoints will reject disabled or banned users.
- A user may only read a conversation if they are a member.
- A user may only send into a conversation if they are an active member.
- Group membership changes must be persisted before realtime events are published.

For the first release, the browser-facing chat UI will use session auth. Token-based access can be added later if external API clients need a chat integration path.

### 3.5 Message flow
A message send request will follow this sequence:
1. Controller validates the request and identifies the current user.
2. Service confirms membership and conversation state.
3. Service writes the message to the database.
4. Service updates unread counters and last-message metadata.
5. Service publishes a realtime event to the conversation channel.
6. Connected clients receive the event and update the UI.

This keeps persistence authoritative and realtime delivery best-effort but fast.

### 3.6 Presence and typing
Presence and typing should be ephemeral and cheap.

- Presence updates will be written to Redis with a short TTL.
- Typing indicators will be broadcast as realtime events and expire automatically on the client.
- No permanent presence history will be stored in the database during the first release.

### 3.7 Database compatibility
All persistent tables must remain compatible with SQLite, MySQL, and PostgreSQL.

Rules:
- Use GORM `AutoMigrate` for new tables.
- Use portable column types like `text`, `varchar`, `int`, and `bigint`.
- Avoid database-specific JSON types for persisted payloads.
- Add indexes where needed for conversation membership, message lookup, and unread counters.
- Keep migration logic in `model/main.go` consistent with existing project patterns.

## 4. Frontend Architecture

### 4.1 Placement
The current `web/default/src/features/chat` area already serves external chat preset links. That feature should remain intact.

The new internal messaging feature should live in a separate feature namespace, such as:
- `web/default/src/features/messaging/`

This avoids mixing internal IM state with external chat-link helpers.

### 4.2 Routes
The authenticated web app will gain new routes for the internal chat UI, such as:
- conversation list
- conversation detail
- group creation / editing
- member management

Recommended route shape:
- `/messages`
- `/messages/:conversationId`
- `/messages/groups/:groupId` if a separate group detail view is useful

### 4.3 Client realtime integration
The web client will connect to the backend realtime endpoint with `centrifuge-js`.

Client responsibilities:
- connect after authentication is ready,
- subscribe to the current user channel,
- subscribe to the active conversation channel,
- merge realtime events into React Query / local UI state,
- keep typing and presence indicators short-lived,
- recover on reconnect.

### 4.4 UI layout
The first release should keep the layout simple:
- left rail: conversations and groups,
- center pane: messages,
- right rail or drawer: members, read status, and group actions.

The UI should reuse the existing design system, sidebar patterns, and authentication state rather than introducing a new visual language.

## 5. Error Handling and Security

### Error handling
- Unauthorized requests must fail early at the controller layer.
- Membership violations must return a clear business error.
- Database errors must be logged without leaking message content or secrets.
- Realtime publish failures should not erase the persisted message; instead, the client should recover from history on reconnect.

### Security
- Do not expose message data to users outside the conversation membership.
- Do not log full message bodies if they may contain sensitive user content unless a specific debugging path explicitly masks them.
- Keep realtime event payloads minimal: ids, timestamps, sender ids, conversation ids, and user-visible message text only when necessary.
- Rate-limit message sends and membership mutations to reduce abuse.

## 6. Testing Strategy

### Backend tests
- Model tests for table creation and query helpers.
- Service tests for membership checks, message creation, unread updates, and group membership changes.
- Controller tests for auth rejection, request validation, and response shape.
- Realtime adapter tests for publish routing and channel authorization.

### Frontend tests
- Unit tests for message list state merging and unread badge behavior.
- Route tests for auth-gated navigation.
- Typecheck, lint, and production build after implementation.

### Verification command set
- `go test ./model ./service ./controller`
- `cd web/default && bun run typecheck`
- `cd web/default && bun run lint`
- `cd web/default && bun run build`

## 7. Rollout Plan

### Phase 1
Implement the persistence model, service layer, and realtime backbone.

### Phase 2
Add HTTP APIs and authenticated WebSocket endpoints.

### Phase 3
Build the React chat experience and wire the realtime client.

### Phase 4
Add tests, polish empty states, and clean up any rough edges.

## 8. Key Risks

- A too-large first release could turn into a full IM platform instead of a lightweight feature.
- Realtime authorization mistakes could leak messages across conversations.
- Presence state can become noisy if TTLs or reconnect handling are not conservative.
- Frontend routing may collide with existing external chat-link routes if the new namespace is not separated clearly.

The implementation should stay focused on simple, durable chat primitives first.

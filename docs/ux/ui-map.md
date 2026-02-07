# vcdeploy Web UI Map

> **Version:** 1.0  
> **Framework:** Go templates + HTMX + Tailwind CSS

This document defines the web UI structure, pages, and their API dependencies.

---

## Table of Contents

1. [Design Principles](#design-principles)
2. [Page Structure](#page-structure)
3. [API Dependencies](#api-dependencies)
4. [Real-time Features](#real-time-features)
5. [Bulk Operations](#bulk-operations)

---

## Design Principles

### 1. API-Driven

All data operations go through the REST API. The UI is a consumer of the API, never bypassing it for direct database access.

```
User Action → HTMX Request → REST API → Response → UI Update
```

### 2. Progressive Enhancement

- Base functionality works without JavaScript
- HTMX provides dynamic updates
- Alpine.js for complex client-side interactions

### 3. Consistent Layout

Every resource page follows the same pattern:
```
┌─────────────────────────────────────────────┐
│ Header: Title + Action Buttons              │
├─────────────────────────────────────────────┤
│ Filters/Search Bar                          │
├─────────────────────────────────────────────┤
│ Data Table with Selection                   │
│ - Checkbox column                           │
│ - Resource columns                          │
│ - Actions column                            │
├─────────────────────────────────────────────┤
│ Pagination                                  │
├─────────────────────────────────────────────┤
│ Bulk Action Bar (shown when items selected) │
└─────────────────────────────────────────────┘
```

### 4. Real-time Updates

- SSE for live data (logs, status, events)
- Polling fallback for browsers without SSE
- Visual indicators for live data

---

## Page Structure

### Navigation

```
┌─ Sidebar ──────────┐
│ Stats              │
│ ───────────────    │
│ Projects           │
│ Deployments        │
│ Agents             │
│   └─ Provision     │
│ ───────────────    │
│ Recipes            │
│   ├─ Components    │
│   ├─ Playbooks     │
│   └─ Activations   │
│ ───────────────    │
│ Security           │
│   ├─ Certificates  │
│   ├─ Credentials   │
│   ├─ SSH Keys      │
│   ├─ Host Keys     │
│   ├─ Jump Servers  │
│   └─ Blocked IPs   │
│ ───────────────    │
│ Settings           │
│   ├─ General       │
│   ├─ Users         │
│   ├─ API Keys      │
│   └─ Audit Log     │
└────────────────────┘
```

### Pages

#### Stats (`/stats`)

**Purpose:** System statistics dashboard with key metrics and recent activity.

**Sections:**
| Section | Description | API Endpoint |
|---------|-------------|--------------|
| Stats Cards | Projects, Agents, Deployments Today, Success Rate | `GET /stats` |
| Deployment Chart | Success/failure trend over 7 days | `GET /stats/deployments?range=7d` |
| Agent Status | Online/offline agent breakdown | `GET /stats/agents` |
| Recent Deployments | Last 10 deployments with status | `GET /deployments?limit=10` |
| Active Deployments | Currently running (live updates) | SSE `/events/stream?types=deployment` |

**Actions:**
- Quick deploy button (opens modal)
- View all links to respective pages

---

#### Projects (`/projects`)

**Purpose:** Manage deployment projects.

**List View:**
| Column | Description |
|--------|-------------|
| ☐ | Selection checkbox |
| Name | Project name (link to detail) |
| Type | Project type badge |
| Repository | Git URL (truncated) |
| Branch | Default branch |
| Last Deploy | Relative time + status |
| Actions | Deploy, Edit, Delete |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/projects` |
| Search | GET | `/projects?search=...` |
| Create | POST | `/projects` |
| Delete | DELETE | `/projects/{name}` |
| Deploy | POST | `/deployments` |
| Bulk Deploy | POST | `/deployments/bulk` |
| Bulk Delete | DELETE | `/projects/bulk` |

**Detail View (`/projects/{name}`):**
| Section | Description | API |
|---------|-------------|-----|
| Overview | Project config, repo, branch | `GET /projects/{name}` |
| Deployment History | Recent deployments | `GET /deployments?project={name}` |
| Webhooks | Configured webhooks | `GET /projects/{name}/webhooks` |
| Health Checks | Health check config | `GET /health-checks?project={name}` |
| Secrets | Project secrets | `GET /secrets?project={name}` |
| Recipe Activation | Active recipe | `GET /recipes/activations?project={name}` |

---

#### Deployments (`/deployments`)

**Purpose:** View and manage deployment history.

**List View:**
| Column | Description |
|--------|-------------|
| ☐ | Selection checkbox |
| ID | Deployment ID (link to detail) |
| Project | Project name |
| Type | Deploy/Rollback badge |
| Status | Status with color indicator |
| Branch | Deployed branch |
| Duration | Time taken |
| Started | Relative timestamp |
| Actions | Logs, Cancel/Retry |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/deployments` |
| Filter | GET | `/deployments?status=...&project=...` |
| Cancel | DELETE | `/deployments/{id}` |
| Retry | POST | `/deployments/{id}/retry` |
| Bulk Cancel | DELETE | `/deployments/bulk` |

**Detail View (`/deployments/{id}`):**
| Section | Description | API |
|---------|-------------|-----|
| Summary | Status, timing, commit | `GET /deployments/{id}` |
| Logs | Live log stream | SSE `/deployments/{id}/logs/stream` |
| Agent Results | Per-agent status | Included in deployment |

**Real-time:**
- Deployment status updates via SSE
- Log streaming via SSE
- Auto-refresh running deployments

---

#### Agents (`/agents`)

**Purpose:** Manage deployment target agents.

**List View:**
| Column | Description |
|--------|-------------|
| ☐ | Selection checkbox |
| Name | Agent name (link to detail) |
| Status | Online/Offline with indicator |
| Groups | Group badges |
| Last Heartbeat | Relative timestamp |
| Version | Agent version |
| Actions | Edit, Provision, Delete |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/agents` |
| Filter | GET | `/agents?status=...&group=...` |
| Update | PUT | `/agents/{id}` |
| Delete | DELETE | `/agents/{id}` |
| Generate Token | POST | `/agents/tokens` |
| Bulk Update | POST | `/agents/bulk` |
| Bulk Delete | DELETE | `/agents/bulk` |

**Detail View (`/agents/{id}`):**
| Section | Description | API |
|---------|-------------|-----|
| Overview | Name, groups, tags, status | `GET /agents/{id}` |
| System Info | CPU, memory, disk | Included in agent |
| Recent Deployments | Deployments on this agent | `GET /deployments?agent={id}` |
| Certificate | TLS certificate info | `GET /certificates/{id}` |

**Real-time:**
- Agent status via SSE
- Heartbeat indicator

---

#### Settings (`/settings`)

**Purpose:** Configure system settings.

**Tabs:**
| Tab | Description | API |
|-----|-------------|-----|
| General | Site name, URL, timezone | `GET/PUT /settings/general` |
| Appearance | Theme, logo | `GET/PUT /settings/appearance` |
| Security | Session timeout, 2FA | `GET/PUT /settings/security` |
| Notifications | Email, Slack, webhooks | `GET/PUT /settings/notifications` |
| Deployments | Timeouts, retries | `GET/PUT /settings/deployments` |

---

#### Users (`/settings/users`)

**Purpose:** Manage user accounts.

**List View:**
| Column | Description |
|--------|-------------|
| Username | User's login name |
| Email | Email address |
| Role | Admin/Operator/Viewer badge |
| 2FA | Enabled indicator |
| Last Login | Relative timestamp |
| Actions | Edit, Reset Password, Delete |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/users` |
| Create | POST | `/users` |
| Update | PUT | `/users/{id}` |
| Delete | DELETE | `/users/{id}` |
| Reset Password | PUT | `/users/{id}/password` |
| Disable 2FA | DELETE | `/users/{id}/totp` |

---

#### API Keys (`/settings/api-keys`)

**Purpose:** Manage API keys for programmatic access.

**List View:**
| Column | Description |
|--------|-------------|
| Name | Key name |
| Created | Creation date |
| Expires | Expiration date |
| Last Used | Last usage timestamp |
| Actions | Revoke |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/api-keys` |
| Create | POST | `/api-keys` |
| Revoke | DELETE | `/api-keys/{id}` |

---

#### Secrets (`/secrets`)

**Purpose:** Manage deployment secrets.

**List View:**
| Column | Description |
|--------|-------------|
| ☐ | Selection checkbox |
| Key | Secret key name |
| Scope | Global/Project badge |
| Project | Project name (if scoped) |
| Updated | Last update timestamp |
| Actions | Edit, Delete |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/secrets` |
| Filter | GET | `/secrets?project=...` |
| Create/Update | POST | `/secrets` |
| Delete | DELETE | `/secrets/{scope}/{key}` |
| Bulk Import | POST | `/secrets/bulk` |

---

#### Certificates (`/security/certificates`)

**Purpose:** Manage agent certificates and TLS.

**Tabs:**
| Tab | Description | API |
|-----|-------------|-----|
| Agent Certs | List of agent certificates | `GET /certificates` |
| CA | CA certificate info | `GET /certificates/ca` |
| TLS | HTTPS configuration | `GET /tls` |
| Audit | Certificate operations log | `GET /certificates/audit` |

**Actions:**
| Action | API |
|--------|-----|
| Revoke Certificate | `DELETE /certificates/{agent-id}` |
| Rotate CA | `POST /certificates/ca/rotate` |
| Force TLS Renewal | `POST /tls/renew` |

---

#### Credentials (`/security/credentials`)

**Purpose:** Manage Git credentials.

**List View:**
| Column | Description |
|--------|-------------|
| Name | Credential name |
| Type | Token/Basic/SSH badge |
| URL Pattern | Matched repositories |
| Created | Creation date |
| Actions | Test, Edit, Delete |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/credentials` |
| Create | POST | `/credentials` |
| Update | PUT | `/credentials/{id}` |
| Delete | DELETE | `/credentials/{id}` |
| Test | POST | `/credentials/{id}/test` |

---

#### SSH Keys (`/security/ssh-keys`)

**Purpose:** Manage SSH keys for repository access and provisioning.

**List View:**
| Column | Description |
|--------|-------------|
| Name | Key name |
| Type | RSA/Ed25519 badge |
| Fingerprint | Key fingerprint (truncated) |
| Has Passphrase | Yes/No indicator |
| Created | Creation date |
| Actions | Copy Public Key, Delete |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/ssh-keys` |
| Generate | POST | `/ssh-keys/generate` |
| Import | POST | `/ssh-keys` |
| Delete | DELETE | `/ssh-keys/{id}` |
| Show Public | GET | `/ssh-keys/{id}` |

**Create Modal:**
- Tab: Generate New (name, type, passphrase)
- Tab: Import Existing (name, private key, passphrase)

---

#### Host Keys (`/security/host-keys`)

**Purpose:** Manage known host keys for SSH verification.

**List View:**
| Column | Description |
|--------|-------------|
| Host | Hostname/IP |
| Key Type | RSA/Ed25519/ECDSA |
| Fingerprint | Key fingerprint |
| Added | When added |
| Actions | Delete |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/host-keys` |
| Scan | POST | `/host-keys/scan` |
| Create | POST | `/host-keys` |
| Delete | DELETE | `/host-keys/{id}` |

**Create Modal:**
- Tab: Scan Host (hostname, port)
- Tab: Add Manually (hostname, key type, key data)

---

#### Jump Servers (`/security/jump-servers`)

**Purpose:** Manage SSH jump/bastion servers.

**List View:**
| Column | Description |
|--------|-------------|
| Name | Server name |
| Host | Hostname:port |
| User | SSH username |
| Key | Associated SSH key |
| Status | Connected/Error badge |
| Actions | Test, Edit, Delete |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/jump-servers` |
| Create | POST | `/jump-servers` |
| Update | PUT | `/jump-servers/{id}` |
| Delete | DELETE | `/jump-servers/{id}` |
| Test | POST | `/jump-servers/{id}/test` |

---

#### Provision Jobs (`/agents/provision`)

**Purpose:** Manage agent provisioning jobs.

**List View:**
| Column | Description |
|--------|-------------|
| ID | Job ID (link to detail) |
| Target | Host:port |
| Status | Pending/Running/Success/Failed |
| Agent | Resulting agent name |
| Started | Start timestamp |
| Duration | Time taken |
| Actions | Logs, Cancel |

**API Endpoints:**
| Action | Method | Endpoint |
|--------|--------|----------|
| List | GET | `/provision-jobs` |
| Create | POST | `/provision-jobs` |
| Cancel | DELETE | `/provision-jobs/{id}` |
| Show | GET | `/provision-jobs/{id}` |
| Logs | GET | `/provision-jobs/{id}/logs` |

**Detail View (`/agents/provision/{id}`):**
| Section | Description | API |
|---------|-------------|-----|
| Summary | Status, target, config | `GET /provision-jobs/{id}` |
| Logs | Live log stream | SSE `/provision-jobs/{id}/logs/stream` |
| Resulting Agent | Link to agent (if success) | Included in job |

**Create Modal:**
- Target host and port
- SSH user and key selection
- Jump server (optional)
- Agent name and groups
- Binary version selection

---

#### Recipes - Components (`/recipes/components`)

**List View:**
| Column | Description |
|--------|-------------|
| Name | Component name |
| Slug | URL slug |
| Description | Brief description |
| Used In | Playbook count |
| Actions | Edit, Delete |

---

#### Recipes - Playbooks (`/recipes/playbooks`)

**List View:**
| Column | Description |
|--------|-------------|
| Name | Playbook name |
| Slug | URL slug |
| Steps | Number of steps |
| Activations | Active project count |
| Actions | Edit, Compose, Delete |

**Composer View (`/recipes/playbooks/{slug}/compose`):**
- Drag-and-drop step editor
- Component library sidebar
- Variable configuration
- Preview mode

---

#### Audit Log (`/settings/audit`)

**Purpose:** View system audit trail.

**List View:**
| Column | Description |
|--------|-------------|
| Timestamp | Event time |
| User | Acting user |
| Action | Action type |
| Resource | Affected resource |
| Details | Action details |

**Filters:**
- User
- Action type
- Resource type
- Date range

**Export:**
- CSV download
- JSON download

---

## Real-time Features

### SSE Endpoints Used

| Feature | Endpoint | Events |
|---------|----------|--------|
| Dashboard | `/events/stream?types=deployment,agent` | Deployment status, agent online/offline |
| Deployment Logs | `/deployments/{id}/logs/stream` | Log lines |
| Provision Logs | `/provision-jobs/{id}/logs/stream` | Log lines |
| Agent Status | `/events/stream?types=agent` | Agent heartbeats |

### Implementation Pattern

```html
<!-- HTMX SSE Extension -->
<div hx-ext="sse" 
     sse-connect="/api/v1/events/stream?types=deployment"
     sse-swap="deployment">
  <!-- Content updated on deployment events -->
</div>
```

### Fallback

For browsers without SSE support or when SSE fails:
- Poll every 5 seconds for active operations
- Visual indicator shows "Live" vs "Polling" mode

---

## Bulk Operations

### Selection Pattern

```html
<table>
  <thead>
    <tr>
      <th><input type="checkbox" id="select-all"></th>
      ...
    </tr>
  </thead>
  <tbody>
    <tr>
      <td><input type="checkbox" class="row-select" value="{id}"></td>
      ...
    </tr>
  </tbody>
</table>

<!-- Bulk action bar (hidden until selection) -->
<div id="bulk-actions" class="hidden">
  <span id="selected-count">0</span> selected
  <button hx-delete="/api/v1/{resource}/bulk" 
          hx-include=".row-select:checked">
    Delete Selected
  </button>
</div>
```

### Available Bulk Operations

| Resource | Operations |
|----------|------------|
| Projects | Delete, Deploy |
| Deployments | Cancel |
| Agents | Update (groups), Delete |
| Secrets | Delete, Import |

---

## Forms & Validation

### Pattern

```html
<form hx-post="/api/v1/projects" 
      hx-ext="json-enc"
      hx-target="#result"
      hx-swap="innerHTML">
  
  <input name="name" required pattern="[a-z0-9-]+">
  <span class="error" data-error="name"></span>
  
  <button type="submit">Create</button>
</form>
```

### Error Handling

API errors are displayed inline:
```javascript
// HTMX error handler
document.body.addEventListener('htmx:responseError', function(evt) {
  const error = JSON.parse(evt.detail.xhr.responseText);
  // Display error.message or field-specific errors
});
```

---

## Modals

### Pattern

```html
<!-- Trigger -->
<button hx-get="/partials/modal/create-project"
        hx-target="#modal-container"
        hx-swap="innerHTML">
  New Project
</button>

<!-- Modal container -->
<div id="modal-container"></div>
```

### Modal Partial

```html
<div class="modal-backdrop" onclick="closeModal()">
  <div class="modal" onclick="event.stopPropagation()">
    <header>
      <h2>Create Project</h2>
      <button onclick="closeModal()">×</button>
    </header>
    <form ...>
      ...
    </form>
  </div>
</div>
```

---

## Accessibility

- All interactive elements are keyboard accessible
- ARIA labels on icons and buttons
- Color is not the only status indicator
- Focus management in modals
- Screen reader announcements for async updates
---

## Appendix: Additional Pages

### Analytics Dashboard (`/analytics`)

**Purpose:** Detailed analytics and trends view.

**Components:**
| Component | API Endpoint | Update |
|-----------|--------------|--------|
| Deployment Trend Chart | `GET /stats/deployments?range=30d` | Refresh on range change |
| Success Rate Gauge | `GET /stats/deployments?range=7d` | Cached 5min |
| Agent Utilization | `GET /stats/agents` | SSE |
| Top Projects Table | `GET /stats/deployments?group_by=project` | Refresh on range change |
| Failure Breakdown | `GET /stats/deployments?status=failed` | Refresh |

**Features:**
- Date range picker (7d, 30d, 90d, custom)
- Export chart data as CSV
- Click-through to filtered deployment list

### Scheduled Deployments (`/deployments/scheduled`)

**Purpose:** View and manage scheduled deployments.

**Components:**
| Component | API Endpoint | Update |
|-----------|--------------|--------|
| Scheduled Queue | `GET /deployments?status=scheduled` | SSE |
| Calendar View | Same endpoint | On navigation |
| Edit Modal | `PUT /deployments/{id}` | On submit |

**Features:**
- Calendar and list view toggle
- Edit scheduled time
- Cancel scheduled deployment
- Pause/resume scheduling

---

## Appendix: UI/UX Improvements

### Skeleton Loading States

While data loads, show animated skeleton placeholders:

```html
<div class="skeleton">
  <div class="skeleton-row"></div>
  <div class="skeleton-row"></div>
  <div class="skeleton-row"></div>
</div>
```

```css
.skeleton-row {
  background: linear-gradient(90deg, #f0f0f0 25%, #e0e0e0 50%, #f0f0f0 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}
```

### Keyboard Shortcuts

| Shortcut | Action | Scope |
|----------|--------|-------|
| `?` | Show keyboard shortcuts | Global |
| `/` | Focus search | Global |
| `g p` | Go to Projects | Global |
| `g d` | Go to Deployments | Global |
| `g a` | Go to Agents | Global |
| `n` | New (context-aware) | List pages |
| `j` / `k` | Navigate rows | Tables |
| `x` | Toggle selection | Tables |
| `Esc` | Close modal/deselect | Modals, tables |

### Toast Notifications

For non-blocking feedback:

```html
<div id="toast-container" class="fixed bottom-4 right-4 space-y-2">
  <!-- Toasts appear here -->
</div>
```

Types:
- **Success:** Green, auto-dismiss 3s
- **Error:** Red, manual dismiss
- **Warning:** Yellow, auto-dismiss 5s
- **Info:** Blue, auto-dismiss 3s

### Mobile Responsiveness

| Breakpoint | Layout Changes |
|------------|----------------|
| `< 768px` | Sidebar collapses to hamburger menu |
| `< 768px` | Tables become card view |
| `< 768px` | Bulk action bar moves to bottom |
| `< 1024px` | Dashboard cards stack vertically |
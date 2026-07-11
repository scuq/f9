# Import map scripts (Lua)

Map scripts let you shape imported sessions with code. On every **test** and
**refresh** of an import source, each decoded record runs through your script's
`map(r)` — after the source's generic filter, before reconcile:

    fetch → decode → filter (rule groups) → map(r) → reconcile into the tree

Scripts live in a global library (**Settings → import map scripts**) and are
selected per source in the import dialog. They are plain Lua 5.1 (pure-Go
runtime), so a script is shareable: teammates import the same script and only
maintain their own usernames (see *alternative usernames* below).

## The contract

Define one function:

```lua
function map(r)
  -- inspect / modify r
  return r    -- keep the (possibly modified) record
  -- or: return nil to drop it
end
```

`r` is one imported record:

| field      | type    | meaning |
|------------|---------|---------|
| `r.name`   | string  | session name (must stay unique per target folder) |
| `r.host`   | string  | host/IP (NetBox: the primary IP, already extracted) |
| `r.port`   | number  | SSH port (0 → default 22) |
| `r.user`   | string  | login on the target; wins over a jump chain's shell-hop user override |
| `r.proto`  | string  | protocol (`ssh`) |
| `r.folder` | string  | nested folder path under the source folder, e.g. `"site/role"`; `""` = the source folder itself |
| `r.tags`   | table   | list of tags |
| `r.attrs`  | table   | normalized attributes: `status`, `role`, `site`, `tenant`, `manufacturer`, `model`, `hostname`; NetBox custom fields as `attrs["cf:<name>"]` |
| `r.raw`    | table   | the full decoded source object (NetBox: the device JSON), for anything `attrs` doesn't cover |

Behavior notes:

- **Folders are real.** `r.folder` paths (max depth 8; `.`/`..` rejected)
  create nested folders under the source folder. Sessions move when their path
  changes between refreshes, and auto-created folders that become empty are
  pruned. Hand-made folders are never pruned. Name uniqueness is **per leaf
  folder**, so the same device name may exist under different sites.
- **Users and labels.** Setting `r.user = "@somelabel"` stores a reference to an
  *alternative username* (Settings): it resolves to that user's login at
  connect time, and the label's optional key file joins the auth candidates.
  `f9.alt_user("somelabel")` returns the resolved login (or `nil`) if you need
  the literal value inside the script. Leaving `r.user` empty lets a jump
  chain's shell-hop user override apply as the fallback.
- **Sandbox.** Scripts get the Lua base, `table`, `string` and `math` libraries
  only — no `os`, no `io`, no file loading — and run under a deadline. An error
  (or a non-table return) aborts the import with the record's name in the
  message.
- **Test vs refresh.** The test button fetches only until it has a
  representative sample of matches and marks partial results as a preview;
  refresh processes the whole source. A refresh that decodes zero records
  leaves the existing tree untouched.

## Examples

### 1. Filter and rename

Keep only active devices of a given role, name sessions `hostname_ip`, and tag
them:

```lua
function map(r)
  if r.attrs.status ~= "active" then return nil end
  if r.attrs.role ~= "access-switch" then return nil end
  if r.host == "" then return nil end          -- no primary IP -> unconnectable

  r.name = (r.attrs.hostname or r.name) .. "_" .. r.host
  r.tags = { "imported", "switch" }
  return r
end
```

### 2. A folder tree from attributes

Build `<tenant>/<site>/<role>` under the source folder; route one special role
to its own top-level folder:

```lua
function map(r)
  if r.host == "" then return nil end

  if r.attrs.role == "bastion" then
    r.folder = "00-BASTIONS"
    return r
  end

  local tenant = r.attrs.tenant or "no-tenant"
  local site   = r.attrs.site   or "no-site"
  local role   = r.attrs.role   or "no-role"
  r.folder = tenant .. "/" .. site .. "/" .. role
  return r
end
```

### 3. Custom fields and per-device logins

Filter on a NetBox custom field and pick the login per device class using
alternative-username labels — the script stays free of real usernames:

```lua
function map(r)
  -- keep devices whose custom field "deviceClass" starts with NET_
  local class = r.attrs["cf:deviceClass"] or ""
  if string.sub(class, 1, 4) ~= "NET_" then return nil end
  if r.host == "" then return nil end

  r.folder = (r.attrs.site or "no-site") .. "/" .. (r.attrs.role or "no-role")

  -- login per management profile (labels resolve at connect time)
  local profile = r.attrs["cf:mgmtProfile"] or ""
  if profile == "TACACS" then
    r.user = "@tacacs"
  elseif profile == "LOCAL_ADMIN" then
    r.user = "admin"
  end
  -- otherwise leave r.user empty: the source jump chain's shell-hop
  -- override (if any) applies as the fallback login.

  return r
end
```

## Debugging tips

- **Discover custom-field keys**: temporarily replace the script body with

  ```lua
  function map(r)
    local keys = ""
    for k, _ in pairs(r.raw.custom_fields or {}) do keys = keys .. k .. " " end
    r.name = keys
    return r
  end
  ```

  and hit **test connection** — the sample names list every key. The same trick
  works for any value (`r.name = tostring(r.attrs["cf:whatever"])`).
- **Zero matches on test** may just mean your matches sort late in the source:
  the preview scans a bounded prefix and says how much it covered. Refresh
  scans everything.
- A dropped record is silent by design; to see *why* something is dropped, turn
  the `return nil` branches into `r.name = "DROP: <reason>"` temporarily.

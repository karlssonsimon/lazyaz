local Explorer = require("aztools.explorer")

local M = {}
local instance

local function field(item, ...)
  if type(item) ~= "table" then
    return nil
  end
  for i = 1, select("#", ...) do
    local key = select(i, ...)
    local value = item[key]
    if value ~= nil then
      return value
    end
  end
  return nil
end

local function panes(snapshot)
  snapshot = snapshot or {}
  local out = { { key = "subscriptions", title = "Subscriptions", items = snapshot.subscriptions or {} } }
  if snapshot.has_subscription then
    out[#out + 1] = { key = "vaults", title = "Vaults", items = snapshot.vaults or {} }
  end
  if snapshot.has_vault then out[#out + 1] = { key = "secrets", title = "Secrets", items = snapshot.secrets or {} } end
  if snapshot.has_secret then out[#out + 1] = { key = "versions", title = "Versions", items = snapshot.versions or {} } end
  if snapshot.preview_open then
    local text = snapshot.preview_value or ""
    local preview_lines = text ~= "" and vim.split((text:gsub("\r", "")), "\n", { plain = true }) or {}
    local title = field(snapshot.current_secret, "name", "Name") or "Secret"
    if snapshot.preview_version and snapshot.preview_version ~= "" then
      title = title .. "@" .. snapshot.preview_version
    end
    out[#out + 1] = { key = "preview", title = title, items = preview_lines, preview = true }
  end
  return out
end

local function icon_info(name, kind)
  if _G.MiniIcons ~= nil then
    local category = kind == "dir" and "directory" or "file"
    local icon, hl = _G.MiniIcons.get(category, name)
    return icon, hl
  end
  if kind == "dir" then return "", vim.fn.hlexists("MiniFilesDirectory") == 1 and "MiniFilesDirectory" or "Directory" end
  return "", vim.fn.hlexists("MiniFilesFile") == 1 and "MiniFilesFile" or "Normal"
end

local adapter = {
  panes = panes,
  pane_filetype = function(key)
    if key == "preview" then
      return "text"
    end
    return "azkv"
  end,
  item_prefix = function(pane_key, item)
    if pane_key == "preview" or type(item) ~= "table" then return nil, nil end
    local label = field(item, "name", "Name", "id", "ID", "version", "Version") or tostring(item)
    local kind = pane_key == "versions" and "file" or "dir"
    return icon_info(label, kind)
  end,
  item_label = function(_, item)
    local label = field(item, "name", "Name", "id", "ID", "version", "Version") or tostring(item)
    return label
  end,
  item_name_highlight = function(pane_key)
    if pane_key == "subscriptions" or pane_key == "vaults" or pane_key == "secrets" then
      return "Directory"
    end
  end,
  should_bootstrap = function(snapshot)
    snapshot = snapshot or {}
    return not snapshot.loading and (type(snapshot.subscriptions) ~= "table" or #snapshot.subscriptions == 0)
  end,
  refresh_action = function() return { Action = "kv.refresh" } end,
  left_action = function() return { Action = "kv.navigate.left" } end,
  focus_next_action = function() return { Action = "kv.focus.next" } end,
  focus_prev_action = function() return { Action = "kv.focus.previous" } end,
  reveal_action = function(_, pane, item)
    if pane == "subscriptions" then
      return { Action = "kv.select.subscription", Subscription = { ID = field(item, "id", "ID"), Name = field(item, "name", "Name") } }
    elseif pane == "vaults" then
      return { Action = "kv.select.vault", Vault = { Name = field(item, "name", "Name"), SubscriptionID = field(item, "subscription_id", "SubscriptionID"), VaultURI = field(item, "vault_uri", "VaultURI") } }
    elseif pane == "secrets" then
      return { Action = "kv.select.secret", Secret = { Name = field(item, "name", "Name") } }
    end
  end,
  open_action = function(_, pane, item)
    if pane == "subscriptions" then
      return { Action = "kv.select.subscription", Subscription = { ID = field(item, "id", "ID"), Name = field(item, "name", "Name") } }
    elseif pane == "vaults" then
      return { Action = "kv.select.vault", Vault = { Name = field(item, "name", "Name"), SubscriptionID = field(item, "subscription_id", "SubscriptionID"), VaultURI = field(item, "vault_uri", "VaultURI") } }
    elseif pane == "secrets" then
      return { Action = "kv.select.secret", Secret = { Name = field(item, "name", "Name") } }
    elseif pane == "versions" then
      return { Action = "kv.preview.secret", Version = field(item, "version", "Version") }
    end
  end,
}

local function get() if not instance then instance = Explorer:new("kv", adapter) end return instance end

function M.command(action)
  local explorer = get()
  if action == "open" then
    return explorer:toggle()
  end
end

function M.open() get():open() end
function M.toggle() get():toggle() end
function M.close() get():close() end
function M.stop() get():stop() end
return M

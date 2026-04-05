local Explorer = require("aztools.explorer")
local clipboard = require("aztools.clipboard")
local config = require("aztools.config")

local M = {}
local instance

local uv = vim.uv or vim.loop

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

local function icon_info(name, kind)
  if _G.MiniIcons ~= nil then
    local category = kind == "dir" and "directory" or "file"
    local icon, hl = _G.MiniIcons.get(category, name)
    return icon, hl
  end
  if kind == "dir" then return "", vim.fn.hlexists("MiniFilesDirectory") == 1 and "MiniFilesDirectory" or "Directory" end
  return "", vim.fn.hlexists("MiniFilesFile") == 1 and "MiniFilesFile" or "Normal"
end

local function human_size(size)
  local units = { "B", "KB", "MB", "GB", "TB" }
  local value = tonumber(size or 0) or 0
  local idx = 1
  while value >= 1024 and idx < #units do value = value / 1024; idx = idx + 1 end
  if idx == 1 then return string.format("%d %s", value, units[idx]) end
  return string.format("%.1f %s", value, units[idx])
end

local function format_modified(value)
  if not value or value == "" then return nil end
  local ok, dt = pcall(vim.fn.strptime, "%Y-%m-%dT%H:%M:%SZ", value)
  if ok and dt and dt > 0 then return os.date("%Y-%m-%d %H:%M", dt) end
  return value
end

local function download_root(snapshot)
  local root = config.module("blob").download_root
  local account = snapshot and field(snapshot.current_account, "name", "Name") or nil
  local container = snapshot and field(snapshot, "container_name", "ContainerName") or nil
  if not account or account == "" or not container or container == "" then
    return root
  end
  return vim.fs.joinpath(root, account, container)
end

local function staging_root()
  return vim.fs.joinpath(config.get().runtime.cache_root, "blob", "staging", tostring(uv.hrtime()))
end

local function blob_destination(root, blob_name)
  local normalized = tostring(blob_name or ""):gsub("\\", "/")
  normalized = vim.trim(normalized)
  normalized = normalized:gsub("^/+", "")
  if normalized == "" then
    return nil
  end

  local parts = vim.split(normalized, "/", { plain = true, trimempty = true })
  if #parts == 0 then
    return nil
  end
  for _, part in ipairs(parts) do
    if part == "." or part == ".." then
      return nil
    end
  end
  return vim.fs.joinpath(root, unpack(parts)), table.concat(parts, "/")
end

local function notify(msg, level)
  vim.notify(msg, level or vim.log.levels.INFO)
end

local function staged_entries(root, blob_names)
  local entries = {}
  for _, blob_name in ipairs(blob_names) do
    local abs_path, relative_path = blob_destination(root, blob_name)
    if abs_path and uv.fs_stat(abs_path) then
      entries[#entries + 1] = {
        source_path = abs_path,
        relative_path = relative_path,
        blob_name = blob_name,
      }
    end
  end
  return entries
end

local function selected_blob_names(explorer, opts)
  local pane, items = explorer:entries_from_range(opts.line1, opts.line2)
  if not pane or pane.key ~= "blobs" or not items or #items == 0 then
    return nil, "Select one or more blobs first"
  end

  local names = {}
  for _, item in ipairs(items) do
    local name = field(item, "name", "Name")
    local is_prefix = field(item, "is_prefix", "IsPrefix")
    if name and not is_prefix then
      names[#names + 1] = name
    end
  end

  if #names == 0 then
    return nil, "Select one or more blobs first"
  end

  return names
end

local function copy_file(source_path, target_path)
  local target_dir = vim.fs.dirname(target_path)
  if target_dir and target_dir ~= "" then
    local ok = vim.fn.mkdir(target_dir, "p")
    if ok == 0 then
      return false, "Failed to create destination directory " .. target_dir
    end
  end

  local ok, res = pcall(uv.fs_copyfile, source_path, target_path)
  if ok and (res == true or res == nil) then
    return true
  end

  local err = type(res) == "string" and res or (not ok and tostring(res) or "copy failed")
  return false, string.format("Failed to copy %s to %s: %s", source_path, target_path, err)
end

local function yank(explorer, opts)
  local names, err = selected_blob_names(explorer, opts)
  if not names then
    notify(err, vim.log.levels.WARN)
    return
  end

  local root = staging_root()
  clipboard.clear_blob()
  explorer:invoke({ Action = "blob.download", BlobNames = names, DestinationRoot = root, VisibleLines = 20 })

  local entries = staged_entries(root, names)
  if #entries == 0 then
    pcall(vim.fn.delete, root, "rf")
    local level = (explorer.snapshot and explorer.snapshot.last_err and explorer.snapshot.last_err ~= "") and vim.log.levels.ERROR or vim.log.levels.WARN
    notify((explorer.snapshot and explorer.snapshot.last_err and explorer.snapshot.last_err ~= "") and explorer.snapshot.last_err or "No blobs were yanked", level)
    return
  end

  clipboard.set_blob({
    staging_root = root,
    entries = entries,
    account = explorer.snapshot and field(explorer.snapshot.current_account, "name", "Name") or nil,
    container = explorer.snapshot and field(explorer.snapshot, "container_name", "ContainerName") or nil,
  })
  notify(string.format("Yanked %d blob(s)", #entries))
end

local function put()
  local payload = clipboard.get_blob()
  if not payload or type(payload.entries) ~= "table" or #payload.entries == 0 then
    notify("No yanked blobs to put", vim.log.levels.WARN)
    return
  end

  local ok, integration = pcall(require, "aztools.integrations.minifiles")
  if not ok then
    notify("mini.files integration is unavailable", vim.log.levels.ERROR)
    return
  end

  local target_dir, MiniFiles = integration.resolve_target_dir()
  if not target_dir then
    notify(MiniFiles, vim.log.levels.WARN)
    return
  end

  local copied, skipped = 0, 0
  for _, entry in ipairs(payload.entries) do
    local target_path = vim.fs.joinpath(target_dir, unpack(vim.split(entry.relative_path, "/", { plain = true, trimempty = true })))
    if uv.fs_stat(target_path) then
      skipped = skipped + 1
      notify("File already exists: " .. target_path, vim.log.levels.WARN)
    else
      local success, err = copy_file(entry.source_path, target_path)
      if success then
        copied = copied + 1
      else
        notify(err, vim.log.levels.ERROR)
      end
    end
  end

  integration.refresh(MiniFiles)
  clipboard.clear_blob()

  if copied > 0 and skipped > 0 then
    notify(string.format("Put %d blob(s); skipped %d existing file(s)", copied, skipped))
    return
  end
  if copied > 0 then
    notify(string.format("Put %d blob(s)", copied))
    return
  end
  notify("No blobs were put", vim.log.levels.WARN)
end

local function panes(snapshot)
  snapshot = snapshot or {}
  local out = { { key = "subscriptions", title = "Subscriptions", items = snapshot.subscriptions or {} } }
  if snapshot.has_subscription then
    out[#out + 1] = { key = "accounts", title = "Accounts", items = snapshot.accounts or {} }
  end
  if snapshot.has_account then out[#out + 1] = { key = "containers", title = "Containers", items = snapshot.containers or {} } end
  if snapshot.has_container then out[#out + 1] = { key = "blobs", title = string.format("Blobs [%d]", type(snapshot.blobs) == "table" and #snapshot.blobs or 0), items = snapshot.blobs or {} } end
  if snapshot.preview and snapshot.preview.open then
    local text = snapshot.preview.window_text or ""
    local preview_lines = text ~= "" and vim.split((text:gsub("\r", "")), "\n", { plain = true }) or {}
    out[#out + 1] = { key = "preview", title = snapshot.preview.blob_name or "Preview", items = preview_lines, preview = true }
  end
  return out
end

local adapter = {
  panes = panes,
  pane_filetype = function(key, _, snapshot)
    if key ~= "preview" then return "azblob" end
    local name = snapshot and snapshot.preview and snapshot.preview.blob_name or ""
    local base = vim.fs.basename(name)
    return vim.filetype.match({ filename = base }) or vim.filetype.match({ filename = name }) or "text"
  end,
  item_prefix = function(pane_key, item)
    if type(item) ~= "table" then return nil, nil end
    local label = field(item, "name", "Name", "id", "ID") or tostring(item)
    return icon_info(label, (pane_key ~= "blobs" or field(item, "is_prefix", "IsPrefix")) and "dir" or "file")
  end,
  item_label = function(pane_key, item)
    if type(item) == "string" then return item end
    local label = field(item, "name", "Name", "id", "ID") or tostring(item)
    if pane_key ~= "blobs" or field(item, "is_prefix", "IsPrefix") then
      return label .. (field(item, "is_prefix", "IsPrefix") and "/" or "")
    end
    return label
  end,
  item_name_highlight = function(pane_key, item, default_hl)
    if pane_key == "subscriptions" or pane_key == "accounts" or pane_key == "containers" then
      return vim.fn.hlexists("MiniFilesDirectory") == 1 and "MiniFilesDirectory" or "Directory"
    end
    if pane_key == "blobs" and field(item, "is_prefix", "IsPrefix") then
      return vim.fn.hlexists("MiniFilesDirectory") == 1 and "MiniFilesDirectory" or "Directory"
    end
  end,
  item_virtual_lines = function(pane_key, item)
    if pane_key ~= "blobs" or type(item) ~= "table" or item.is_prefix then return nil end
    local parts = {}
    local modified = format_modified(item.last_modified)
    if modified then parts[#parts + 1] = modified end
    local size = field(item, "size", "Size")
    local access_tier = field(item, "access_tier", "AccessTier")
    if size and size > 0 then parts[#parts + 1] = human_size(size) end
    if access_tier and access_tier ~= "" then parts[#parts + 1] = access_tier end
    if #parts == 0 then return nil end
    return { { "  " .. table.concat(parts, "  "), "AztoolsGhostText" } }
  end,
  should_bootstrap = function(snapshot)
    snapshot = snapshot or {}
    return not snapshot.loading and (type(snapshot.subscriptions) ~= "table" or #snapshot.subscriptions == 0)
  end,
  refresh_action = function() return { Action = "blob.refresh" } end,
  left_action = function() return { Action = "blob.navigate.left", HierarchyLimit = 200, PrefixSearchLimit = 200, VisibleLines = 20 } end,
  back_action = function(explorer)
    local pane = explorer:current_pane_key()
    if explorer.snapshot and explorer.snapshot.preview and explorer.snapshot.preview.open and (pane == "preview" or pane == "blobs") then return { Action = "blob.preview.close", VisibleLines = 20 } end
  end,
  reveal_action = function(_, pane, item)
    if pane == "subscriptions" then
      return { Action = "blob.select.subscription", Subscription = { ID = field(item, "id", "ID"), Name = field(item, "name", "Name") }, HierarchyLimit = 200, PrefixSearchLimit = 200, VisibleLines = 20 }
    elseif pane == "accounts" then
      return { Action = "blob.select.account", Account = { Name = field(item, "name", "Name"), SubscriptionID = field(item, "subscription_id", "SubscriptionID"), ResourceGroup = field(item, "resource_group", "ResourceGroup"), BlobEndpoint = field(item, "blob_endpoint", "BlobEndpoint") }, HierarchyLimit = 200, PrefixSearchLimit = 200, VisibleLines = 20 }
    elseif pane == "containers" then
      return { Action = "blob.select.container", ContainerName = field(item, "name", "Name"), HierarchyLimit = 200, PrefixSearchLimit = 200, VisibleLines = 20 }
    end
  end,
  focus_next_action = function() return { Action = "blob.focus.next", VisibleLines = 20 } end,
  focus_prev_action = function() return { Action = "blob.focus.previous", VisibleLines = 20 } end,
  open_action = function(_, pane, item)
    if pane == "subscriptions" then
      return { Action = "blob.select.subscription", Subscription = { ID = field(item, "id", "ID"), Name = field(item, "name", "Name") }, HierarchyLimit = 200, PrefixSearchLimit = 200, VisibleLines = 20 }
    elseif pane == "accounts" then
      return { Action = "blob.select.account", Account = { Name = field(item, "name", "Name"), SubscriptionID = field(item, "subscription_id", "SubscriptionID"), ResourceGroup = field(item, "resource_group", "ResourceGroup"), BlobEndpoint = field(item, "blob_endpoint", "BlobEndpoint") }, HierarchyLimit = 200, PrefixSearchLimit = 200, VisibleLines = 20 }
    elseif pane == "containers" then
      return { Action = "blob.select.container", ContainerName = field(item, "name", "Name"), HierarchyLimit = 200, PrefixSearchLimit = 200, VisibleLines = 20 }
    elseif pane == "blobs" then
      return { Action = "blob.open", Blob = { Name = field(item, "name", "Name"), IsPrefix = field(item, "is_prefix", "IsPrefix"), Size = field(item, "size", "Size") or 0, ContentType = field(item, "content_type", "ContentType") or "" }, HierarchyLimit = 200, PrefixSearchLimit = 200, VisibleLines = 20 }
    end
  end,
}

local function get() if not instance then instance = Explorer:new("blob", adapter) end return instance end

function M.command(action, opts)
  opts = opts or {}
  local explorer = get()
  if action == "open" then
    return explorer:toggle()
  end

  if action == "put" then
    return put()
  end

  if not explorer.proc then
    return
  end

  if action == "refresh" then
    return explorer:invoke(adapter.refresh_action(explorer))
  end

  if action == "loadall" then
    return explorer:invoke({ Action = "blob.toggle_load_all", HierarchyLimit = 200, PrefixSearchLimit = 200, VisibleLines = 20 })
  end

  if action == "download" then
    local names, err = selected_blob_names(explorer, opts)
    if not names then
      notify(err, vim.log.levels.WARN)
      return
    end
    local root = download_root(explorer.snapshot)
    return explorer:invoke({ Action = "blob.download", BlobNames = names, DestinationRoot = root, VisibleLines = 20 })
  end

  if action == "yank" then
    return yank(explorer, opts)
  end
end

function M.open() get():open() end
function M.toggle() get():toggle() end
function M.close() get():close() end
function M.stop() get():stop() end
return M

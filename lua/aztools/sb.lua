local Explorer = require("aztools.explorer")

local M = {}
local instance

local function directory_hl()
  return vim.fn.hlexists("MiniFilesDirectory") == 1 and "MiniFilesDirectory" or "Directory"
end

local function file_hl()
  return vim.fn.hlexists("MiniFilesFile") == 1 and "MiniFilesFile" or "Normal"
end

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
    out[#out + 1] = { key = "namespaces", title = "Namespaces", items = snapshot.namespaces or {} }
  end
  if snapshot.has_namespace then
    local entities = snapshot.entities or {}
    if snapshot.dlq_filter then
      entities = vim.tbl_filter(function(item)
        return (field(item, "dead_letter_count", "DeadLetterCount") or 0) > 0
      end, entities)
    end
    out[#out + 1] = { key = "entities", title = "Entities", items = entities }
  end
  if snapshot.has_entity then
    local detail_items = snapshot.detail_mode == "topic_subscriptions" and (snapshot.topic_subs or {}) or (snapshot.peeked_messages or {})
    local detail_title = snapshot.dead_letter and "Detail [DLQ]" or "Detail [Active]"
    out[#out + 1] = { key = "detail", title = detail_title, items = detail_items }
  end
  if snapshot.viewing_message and snapshot.selected_message then
    local body = snapshot.selected_message.full_body or ""
    local lines = body ~= "" and vim.split(body, "\n", { plain = true }) or { "<empty message body>" }
    out[#out + 1] = { key = "preview", title = snapshot.selected_message.message_id or "Message", items = lines, preview = true }
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

local function entity_icon(item)
  if field(item, "kind", "Kind") == 1 then
    return "󱅫", "DiagnosticWarn"
  end
  return "󰉋", directory_hl()
end

local function json_icon()
  if _G.MiniIcons ~= nil then
    local icon, hl = _G.MiniIcons.get("file", "message.json")
    return icon, hl
  end
  return icon_info("message.json", "file")
end

local adapter = {
  panes = panes,
  pane_filetype = function(key)
    if key == "preview" then return "json" end
    return "azsb"
  end,
  pane_winhl = function(key, _, snapshot, active)
    if key ~= "detail" or not snapshot or not snapshot.dead_letter then
      return nil
    end
    local normal = active and "AztoolsPaneActive" or "AztoolsPane"
    return "Normal:" .. normal .. ",FloatBorder:AztoolsBorderDlq,FloatTitle:AztoolsTitleDlq"
  end,
  item_prefix = function(pane_key, item)
    if type(item) ~= "table" then return nil, nil end
    local label = field(item, "name", "Name", "id", "ID", "message_id", "MessageID") or tostring(item)
    if pane_key == "subscriptions" or pane_key == "namespaces" then
      return icon_info(label, "dir")
    end
    if pane_key == "entities" then
      return entity_icon(item)
    end
    if pane_key == "detail" then
      return json_icon()
    end
    return nil, nil
  end,
  item_label = function(pane_key, item)
    if type(item) == "string" then return item end
    local label = field(item, "name", "Name", "id", "ID", "message_id", "MessageID") or tostring(item)
    if pane_key == "entities" then
      local active = field(item, "active_msg_count", "ActiveMsgCount") or 0
      local dlq = field(item, "dead_letter_count", "DeadLetterCount") or 0
      return string.format("%s [A:%d DLQ:%d]", label, active, dlq)
    end
    return label
  end,
  item_name_highlight = function(pane_key, item)
    if pane_key == "subscriptions" or pane_key == "namespaces" then
      return "Directory"
    end
    if pane_key == "detail" then
      return file_hl()
    end
  end,
  item_extra_highlights = function(pane_key, item, label)
    if pane_key ~= "entities" then
      return nil
    end
    local dlq = field(item, "dead_letter_count", "DeadLetterCount") or 0
    if dlq <= 0 then
      return nil
    end
    local start_col, end_col = label:find("DLQ:" .. tostring(dlq), 1, true)
    if not start_col then
      return nil
    end
    return { { group = "AztoolsDangerText", start_col = start_col - 1, end_col = end_col } }
  end,
  should_bootstrap = function(snapshot)
    snapshot = snapshot or {}
    return not snapshot.loading and (type(snapshot.subscriptions) ~= "table" or #snapshot.subscriptions == 0)
  end,
  refresh_action = function() return { Action = "sb.refresh" } end,
  left_action = function() return { Action = "sb.navigate.left" } end,
  back_action = function(explorer)
    local pane = explorer:current_pane_key()
    if explorer.snapshot and explorer.snapshot.viewing_message and (pane == "preview" or pane == "detail") then return { Action = "sb.close.message" } end
  end,
  focus_next_action = function() return { Action = "sb.focus.next" } end,
  focus_prev_action = function() return { Action = "sb.focus.previous" } end,
  reveal_action = function(_, pane, item)
    if pane == "subscriptions" then
      return { Action = "sb.select.subscription", Subscription = { ID = field(item, "id", "ID"), Name = field(item, "name", "Name") } }
    elseif pane == "namespaces" then
      return { Action = "sb.select.namespace", Namespace = { Name = field(item, "name", "Name"), SubscriptionID = field(item, "subscription_id", "SubscriptionID"), ResourceGroup = field(item, "resource_group", "ResourceGroup"), FQDN = field(item, "fqdn", "FQDN") } }
    elseif pane == "entities" then
      return { Action = "sb.select.entity", Entity = { Name = field(item, "name", "Name"), Kind = field(item, "kind", "Kind"), ActiveMsgCount = field(item, "active_msg_count", "ActiveMsgCount") or 0, DeadLetterCount = field(item, "dead_letter_count", "DeadLetterCount") or 0 } }
    elseif pane == "detail" and field(item, "name", "Name") and not field(item, "message_id", "MessageID") then
      return { Action = "sb.select.topic_sub", TopicSub = { Name = field(item, "name", "Name"), ActiveMsgCount = field(item, "active_msg_count", "ActiveMsgCount") or 0, DeadLetterCount = field(item, "dead_letter_count", "DeadLetterCount") or 0 } }
    end
  end,
  open_action = function(_, pane, item)
    if pane == "subscriptions" then
      return { Action = "sb.select.subscription", Subscription = { ID = field(item, "id", "ID"), Name = field(item, "name", "Name") } }
    elseif pane == "namespaces" then
      return { Action = "sb.select.namespace", Namespace = { Name = field(item, "name", "Name"), SubscriptionID = field(item, "subscription_id", "SubscriptionID"), ResourceGroup = field(item, "resource_group", "ResourceGroup"), FQDN = field(item, "fqdn", "FQDN") } }
    elseif pane == "entities" then
      return { Action = "sb.select.entity", Entity = { Name = field(item, "name", "Name"), Kind = field(item, "kind", "Kind"), ActiveMsgCount = field(item, "active_msg_count", "ActiveMsgCount") or 0, DeadLetterCount = field(item, "dead_letter_count", "DeadLetterCount") or 0 } }
    elseif pane == "detail" then
      if field(item, "message_id", "MessageID") then return { Action = "sb.open.message", Message = { MessageID = field(item, "message_id", "MessageID"), FullBody = field(item, "full_body", "FullBody") or "{}" } } end
      if field(item, "name", "Name") then return { Action = "sb.select.topic_sub", TopicSub = { Name = field(item, "name", "Name"), ActiveMsgCount = field(item, "active_msg_count", "ActiveMsgCount") or 0, DeadLetterCount = field(item, "dead_letter_count", "DeadLetterCount") or 0 } } end
    end
  end,
}

local function get() if not instance then instance = Explorer:new("sb", adapter) end return instance end

function M.command(action, opts)
  opts = opts or {}
  local explorer = get()
  if action == "open" then
    return explorer:toggle()
  end

  if not explorer.proc then
    return
  end

  if action == "refresh" then
    return explorer:invoke(adapter.refresh_action(explorer))
  end

  if action == "active" then
    return explorer:invoke({ Action = "sb.show.active" })
  end
  if action == "dlq" then
    return explorer:invoke({ Action = "sb.show.dlq" })
  end
  if action == "filter" then
    return explorer:invoke({ Action = "sb.toggle.dlq_filter" })
  end

  if action == "requeue" or action == "delete" then
    local pane, items = explorer:entries_from_range(opts.line1, opts.line2)
    if not pane or pane.key ~= "detail" or not items or #items == 0 then
      return
    end
    local ids = {}
    for _, item in ipairs(items) do
      local id = field(item, "message_id", "MessageID")
      if id then
        ids[#ids + 1] = id
      end
    end
    if #ids == 0 then
      return
    end
    if action == "requeue" then
      return explorer:invoke({ Action = "sb.requeue", MessageIDs = ids })
    end
    if #ids > 1 then
      return
    end
    return explorer:invoke({ Action = "sb.delete_duplicate", MessageID = ids[1] })
  end
end

function M.open() get():open() end
function M.toggle() get():toggle() end
function M.close() get():close() end
function M.stop() get():stop() end
return M

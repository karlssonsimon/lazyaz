local config = require("aztools.config")
local process = require("aztools.process")
local rpc = require("aztools.rpc")
local Spinner = require("aztools.spinner")
local window = require("aztools.window")

local SPINNER_ACTIONS = {
  ["blob.download"] = true,
}

local Explorer = {}
Explorer.__index = Explorer

local function clamp(index, count)
  if count <= 0 then return 1 end
  if index < 1 then return 1 end
  if index > count then return count end
  return index
end

local function as_list(value)
  if type(value) == "table" then return value end
  return {}
end

local function truncate(text, width)
  local value = tostring(text or "")
  if width <= 0 then return "" end
  if vim.fn.strdisplaywidth(value) <= width then return value end
  if width == 1 then return "…" end
  return vim.fn.strcharpart(value, 0, width - 1) .. "…"
end

local function fit_height(item_count, max_height)
  local desired = math.max(1, item_count) + 2
  return math.min(desired, max_height or 28)
end

local function text_width(text)
  return vim.fn.strdisplaywidth(tostring(text or ""))
end

local function line_text_start_col(line)
  if type(line) ~= "string" then
    return 0
  end
  local _, _, spaces = line:find("^(%s+)")
  return spaces and #spaces or 0
end

function Explorer:new(name, adapter)
  return setmetatable({
    name = name,
    adapter = adapter,
    proc = nil,
    ui = { panes = {}, active_key = nil, previous_focus = nil },
    snapshot = nil,
    filters = {},
    cursors = {},
    pane_defs = {},
    last_reveal = {},
    rendering = false,
    spinner = Spinner.new(),
  }, self)
end

function Explorer:ensure_process()
  if self.proc then return end
  self.proc = process.start(self.name)
  local created = rpc.create_session(self.proc.socket)
  self.proc.session = created.session
  self.snapshot = created.state
end

function Explorer:sync_state()
  self.snapshot = rpc.state(self.proc.socket, self.proc.session)
end

function Explorer:visible_items(pane)
  local items = as_list(pane.items)
  local filter = self.filters[pane.key]
  if not filter or filter == "" then return items end
  local out = {}
  local needle = string.lower(filter)
  for _, item in ipairs(items) do
    local label = self.adapter.item_label and self.adapter.item_label(pane.key, item) or tostring(item)
    if string.find(string.lower(label), needle, 1, true) then
      out[#out + 1] = item
    end
  end
  return out
end

function Explorer:save_cursors()
  for key, pane in pairs(self.ui.panes or {}) do
    if pane.win and vim.api.nvim_win_is_valid(pane.win) then
      self.cursors[key] = vim.api.nvim_win_get_cursor(pane.win)[1]
    end
  end
end

function Explorer:current_pane_key()
  local current = vim.api.nvim_get_current_win()
  for key, pane in pairs(self.ui.panes or {}) do
    if pane.win == current and vim.api.nvim_win_is_valid(current) then
      return key
    end
  end
  return self.ui.active_key or self.ui.previous_focus or (self.snapshot and self.snapshot.focus) or nil
end

function Explorer:selected_entry()
  local key = self:current_pane_key()
  if not key then return nil, nil end
  local pane = self.ui.panes[key]
  if not pane or not pane.win or not vim.api.nvim_win_is_valid(pane.win) then return nil, nil end
  local line = vim.api.nvim_win_get_cursor(pane.win)[1]
  local item = pane.visible_items and pane.visible_items[line]
  if not item then return nil, nil end
  return pane, item
end

function Explorer:current_pane()
  local key = self:current_pane_key()
  if not key then
    return nil
  end
  return self.ui.panes[key]
end

function Explorer:entries_from_range(line1, line2)
  local pane = self:current_pane()
  if not pane or not pane.visible_items then
    return nil, nil
  end
  local start_line = math.max(1, math.min(line1 or 1, line2 or 1))
  local end_line = math.max(line1 or 1, line2 or 1)
  end_line = math.min(end_line, #pane.visible_items)
  start_line = math.min(start_line, end_line)
  local items = {}
  for i = start_line, end_line do
    items[#items + 1] = pane.visible_items[i]
  end
  return pane, items
end

function Explorer:open()
  self.origin_win = self.origin_win or vim.api.nvim_get_current_win()

  if not self.proc then
    self.snapshot = {
      focus = "subscriptions",
      loading = true,
      status = "Starting " .. self.name .. "...",
      subscriptions = {},
    }
    self:render(true)
    self:ensure_focus("subscriptions")

    vim.schedule(function()
      local ok, err = pcall(function()
        self:ensure_process()
        self:render(true)
        self:ensure_focus((self.snapshot and self.snapshot.focus) or (self.pane_defs[1] and self.pane_defs[1].key) or "subscriptions")

        if self.adapter.should_bootstrap and self.adapter.should_bootstrap(self.snapshot) then
          self:invoke(self.adapter.refresh_action(self))
          self:ensure_focus((self.snapshot and self.snapshot.focus) or (self.pane_defs[1] and self.pane_defs[1].key) or "subscriptions")
        end
      end)
      if not ok then
        self.snapshot = {
          focus = "subscriptions",
          loading = false,
          status = "Failed to start " .. self.name,
          last_err = tostring(err),
          subscriptions = {},
        }
        self:render(true)
        self:ensure_focus("subscriptions")
      end
    end)
    return
  end

  self:render(true)
  local target_key = (self.snapshot and self.snapshot.focus) or (self.pane_defs[1] and self.pane_defs[1].key) or "subscriptions"
  self:ensure_focus(target_key)
  vim.schedule(function()
    if not self.proc then
      return
    end
    self:ensure_focus(target_key)
  end)

  if self.adapter.should_bootstrap and self.adapter.should_bootstrap(self.snapshot) then
    vim.schedule(function()
      if not self.proc then
        return
      end
      self:invoke(self.adapter.refresh_action(self))
      self:ensure_focus((self.snapshot and self.snapshot.focus) or (self.pane_defs[1] and self.pane_defs[1].key) or "subscriptions")
    end)
    self:ensure_focus((self.snapshot and self.snapshot.focus) or "subscriptions")
    return
  end
  self:ensure_focus((self.snapshot and self.snapshot.focus) or (self.pane_defs[1] and self.pane_defs[1].key))
end

function Explorer:toggle()
  local any = false
  for _, pane in pairs(self.ui.panes or {}) do
    if pane.win and vim.api.nvim_win_is_valid(pane.win) then any = true break end
  end
  if any then self:close() return end
  self:open()
end

function Explorer:close()
  local wins = vim.api.nvim_tabpage_list_wins(0)
  for i = #wins, 1, -1 do
    local win = wins[i]
    local buf = vim.api.nvim_win_get_buf(win)
    if vim.b[buf].aztools_owned then pcall(vim.api.nvim_win_close, win, true) end
  end
  if self.origin_win and vim.api.nvim_win_is_valid(self.origin_win) then
    pcall(vim.api.nvim_set_current_win, self.origin_win)
  end
  self.ui = { panes = {}, active_key = nil, previous_focus = nil }
  self.last_reveal = {}
  if self.proc and self.proc.session then
    pcall(rpc.close_session, self.proc.socket, self.proc.session)
  end
  process.stop(self.proc)
  self.proc = nil
  vim.cmd("redraw!")
end

function Explorer:stop()
  self:close()
end

function Explorer:invoke(action)
  return self:invoke_with_opts(action, {})
end

function Explorer:invoke_with_opts(action, opts)
  local action_name = type(action) == "table" and action.Action or nil
  local show_spinner = action_name and SPINNER_ACTIONS[action_name] or false
  if show_spinner then self.spinner:start_spinner(action_name) end
  local result = rpc.action(self.proc.socket, self.proc.session, action)
  self.snapshot = result.state
  self:render(not opts.keep_focus)
  if not opts.keep_focus then
    self:ensure_focus((self.snapshot and self.snapshot.focus) or (self.pane_defs[1] and self.pane_defs[1].key))
  end
  if show_spinner then
    local status = self.snapshot and self.snapshot.status or "Done"
    local level = (self.snapshot and self.snapshot.last_err and self.snapshot.last_err ~= "") and vim.log.levels.ERROR or vim.log.levels.INFO
    self.spinner:stop_spinner(status, level)
  end
  return result
end

function Explorer:focus_pane(key)
  local pane = self.ui.panes[key]
  local win = pane and pane.win or self:find_pane_window(key)
  if win and vim.api.nvim_win_is_valid(win) then
    window.focus(win)
    self.ui.active_key = key
    self.ui.previous_focus = key
  end
end

function Explorer:find_pane_window(key)
  if not key then
    return nil
  end
  for _, win in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
    local ok, pane_key = pcall(vim.api.nvim_win_get_var, win, "aztools_pane_key")
    if ok and pane_key == key then
      return win
    end
  end
  return nil
end

function Explorer:ensure_focus(key)
  if not key then
    return
  end
  local attempts = 0
  local timer = vim.loop.new_timer()
  local function stop_timer()
    if timer and not timer:is_closing() then
      timer:stop()
      timer:close()
    end
  end
  timer:start(0, 25, vim.schedule_wrap(function()
    attempts = attempts + 1
    self:focus_pane(key)
    local pane = self.ui.panes[key]
    if pane and pane.win and vim.api.nvim_win_is_valid(pane.win) and vim.api.nvim_get_current_win() == pane.win then
      stop_timer()
      return
    end
    if attempts >= 80 then
      stop_timer()
    end
  end))
end

function Explorer:move_cursor(delta)
  local key = self:current_pane_key()
  if not key then return end
  local pane = self.ui.panes[key]
  if not pane or not pane.win or not vim.api.nvim_win_is_valid(pane.win) then return end
  local max_line = math.max(1, #(pane.visible_items or {}))
  local line = vim.api.nvim_win_get_cursor(pane.win)[1]
  local target = clamp(line + delta, max_line)
  local target_line = vim.api.nvim_buf_get_lines(pane.buf, target - 1, target, false)[1]
  local target_col = pane.key == "preview" and 0 or line_text_start_col(target_line)
  vim.api.nvim_win_set_cursor(pane.win, { target, target_col })
end

function Explorer:open_selected()
  local pane, item = self:selected_entry()
  if not pane or not item then return end
  local action = self.adapter.open_action(self, pane.key, item)
  if action then self:invoke(action) end
end

function Explorer:on_cursor_moved(buf)
  if self.rendering then return end
  local pane_key
  for key, pane in pairs(self.ui.panes or {}) do
    if pane.buf == buf then pane_key = key break end
  end
  if not pane_key then return end
  self.ui.active_key = pane_key
  self.ui.previous_focus = pane_key
  if not self.adapter.live_reveal then return end
  if not self.adapter.reveal_action then return end
  local pane, item = self:selected_entry()
  if not pane or not item then return end
  local identity = pane.key .. ":" .. (type(item) == "table" and (item.name or item.id or item.message_id or tostring(item)) or tostring(item))
  if self.last_reveal[pane.key] == identity then return end
  local action = self.adapter.reveal_action(self, pane.key, item)
  if not action then return end
  self.last_reveal[pane.key] = identity
  self:invoke_with_opts(action, { keep_focus = true })
end

function Explorer:prompt_filter()
  local key = self:current_pane_key()
  local pane = key and self.ui.panes[key] or nil
  if not pane then return end
  vim.ui.input({ prompt = string.format("Filter %s: ", pane.title), default = self.filters[key] or "" }, function(value)
    if value == nil then return end
    self.filters[key] = value ~= "" and value or nil
    self:render(false)
    self:focus_pane(key)
  end)
end

local function clear_preview_ns(buf)
  local ns = vim.api.nvim_create_namespace("aztools-preview")
  vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
  return ns
end

local function add_match(buf, ns, group, line, start_col, end_col)
  pcall(vim.api.nvim_buf_add_highlight, buf, ns, group, line, start_col, end_col)
end

local function highlight_xml(buf, lines)
  local ns = clear_preview_ns(buf)
  for i, line in ipairs(lines) do
    local lnum = i - 1
    local pos = 1
    while true do
      local s, e = line:find("</?[%w:_%-%.]+", pos)
      if not s then break end
      add_match(buf, ns, "AztoolsPreviewTag", lnum, s - 1, e)
      pos = e + 1
    end
    pos = 1
    while true do
      local s, e = line:find('"[^"]*"', pos)
      if not s then break end
      add_match(buf, ns, "AztoolsPreviewString", lnum, s - 1, e)
      pos = e + 1
    end
  end
end

local function highlight_json(buf, lines)
  local ns = clear_preview_ns(buf)
  for i, line in ipairs(lines) do
    local lnum = i - 1
    local s, e = line:find('"[^"]*"%s*:')
    if s then add_match(buf, ns, "AztoolsPreviewKey", lnum, s - 1, e - 1) end
    local pos = 1
    while true do
      local a, b = line:find('"[^"]*"', pos)
      if not a then break end
      add_match(buf, ns, "AztoolsPreviewString", lnum, a - 1, b)
      pos = b + 1
    end
  end
end

function Explorer:highlight_preview(buf, ft, lines)
  if ft == "xml" then
    highlight_xml(buf, lines)
  elseif ft == "json" then
    highlight_json(buf, lines)
  else
    clear_preview_ns(buf)
  end
end

function Explorer:layout_panes(defs)
  local spec = config.module(self.name)
  local area = window.origin()
  local gap = 1
  local focus_key = self:current_pane_key() or (self.snapshot and self.snapshot.focus) or (defs[1] and defs[1].key)
  local max_height = spec.max_height or 28
  local available_width = math.max(40, vim.o.columns - area.col - 2)
  local widths = {}
  local has_preview = false
  for _, def in ipairs(defs) do
    if def.preview then has_preview = true break end
  end
  local preview_width_cap = has_preview and math.max(30, math.floor(available_width * 0.35)) or nil

  for i, def in ipairs(defs) do
    local width = spec.width_nofocus or 22
    if def.preview then
      width = preview_width_cap or spec.width_preview or 60
    elseif def.key == focus_key then
      width = has_preview and math.max(spec.width_nofocus or 22, math.floor((spec.width_focus or 42) * 0.6)) or (spec.width_focus or 42)
    end
    local max_text = text_width(def.title)
    local label_fn = self.adapter.item_label
    for _, entry in ipairs(def.visible_items) do
      local label = label_fn and label_fn(def.key, entry) or tostring(entry)
      max_text = math.max(max_text, text_width(label))
    end
    local width_cap = def.preview and (preview_width_cap or spec.width_preview or 100) or math.max(spec.width_focus or 42, spec.width_nofocus or 22, has_preview and 48 or 100)
    width = math.min(math.max(width, max_text + 2), width_cap)
    widths[i] = width
  end

  local total_width = 0
  for _, w in ipairs(widths) do total_width = total_width + w end
  total_width = total_width + math.max(0, #widths - 1) * gap

  if total_width > available_width then
    local overflow = total_width - available_width
    for i = 1, #defs do
      if overflow <= 0 then break end
      if defs[i].preview then goto continue end
      local min_width = 18
      local shrink = math.min(overflow, math.max(0, widths[i] - min_width))
      widths[i] = widths[i] - shrink
      overflow = overflow - shrink
      ::continue::
    end
    if overflow > 0 then
      for i = 1, #defs do
        if overflow <= 0 then break end
        local min_width = defs[i].preview and 30 or 18
        local shrink = math.min(overflow, math.max(0, widths[i] - min_width))
        widths[i] = widths[i] - shrink
        overflow = overflow - shrink
      end
    end
  end

  local col = area.col
  local layout = {}
  for i, def in ipairs(defs) do
    layout[#layout + 1] = {
      key = def.key,
      title = def.title,
      rect = { row = area.row, col = col, width = widths[i], height = fit_height(#def.visible_items, max_height) },
      pane = def,
    }
    col = col + widths[i] + gap
  end
  return layout
end

function Explorer:render(force_focus)
  if not self.snapshot then return end
  self.rendering = true
  self:save_cursors()
  local previous_focus = self:current_pane_key()
  self.ui.active_key = previous_focus
  local target_key = force_focus and (self.snapshot.focus or (self.pane_defs[1] and self.pane_defs[1].key)) or previous_focus

  local defs = {}
  for _, def in ipairs(self.adapter.panes(self.snapshot) or {}) do
    def.items = as_list(def.items)
    def.visible_items = self:visible_items(def)
    defs[#defs + 1] = def
  end
  self.pane_defs = defs
  local layout = self:layout_panes(defs)
  local keep = {}
  local ordered_layout = {}
  local active_layout = nil

  for _, item in ipairs(layout) do
    if item.key == target_key then
      active_layout = item
    else
      ordered_layout[#ordered_layout + 1] = item
    end
  end
  if active_layout then
    ordered_layout[#ordered_layout + 1] = active_layout
  else
    ordered_layout = layout
  end

  for _, item in ipairs(ordered_layout) do
    local existing = self.ui.panes[item.key]
    local pane_filetype = "aztools." .. self.name
    if self.adapter.pane_filetype then
      pane_filetype = self.adapter.pane_filetype(item.key, item.pane, self.snapshot) or pane_filetype
    end
    local active = ((force_focus and (self.snapshot.focus == item.key)) or ((not force_focus) and previous_focus == item.key))
    local winhl = active and "Normal:AztoolsPaneActive,FloatBorder:AztoolsBorderActive,FloatTitle:AztoolsTitleActive" or "Normal:AztoolsPane,FloatBorder:AztoolsBorder,FloatTitle:AztoolsTitle"
    if self.adapter.pane_winhl then
      winhl = self.adapter.pane_winhl(item.key, item.pane, self.snapshot, active) or winhl
    end
    local pane = window.ensure_pane(existing, item.rect, {
      title = item.title,
      filetype = pane_filetype,
      preview = item.key == "preview",
      winhl = winhl,
      pane_key = item.key,
    })
    pane.key = item.key
    pane.title = item.title
    pane.visible_items = item.pane.visible_items
    self.ui.panes[item.key] = pane
    keep[item.key] = true

    local lines, highlights, prefixes = {}, {}, {}
    for _, entry in ipairs(item.pane.visible_items) do
      local raw = self.adapter.item_label and self.adapter.item_label(item.key, entry) or tostring(entry)
      local prefix_text, prefix_hl = nil, nil
      if self.adapter.item_prefix then
        prefix_text, prefix_hl = self.adapter.item_prefix(item.key, entry)
      end
      prefixes[#prefixes + 1] = { text = prefix_text, hl = prefix_hl }
      local prefix_width = (prefix_text and prefix_text ~= "") and vim.fn.strdisplaywidth(prefix_text .. " ") or 0
      local available_width = item.rect.width - 2 - prefix_width
      lines[#lines + 1] = truncate(raw, available_width)
      if item.key ~= "preview" and (type(entry) == "table" or type(entry) == "string") then
        local label = lines[#lines]
        local hl = prefix_hl
        local name_hl = self.adapter.item_name_highlight and self.adapter.item_name_highlight(item.key, entry, hl) or nil
        if name_hl then
          highlights[#highlights + 1] = { line = #lines - 1, group = name_hl, start_col = line_text_start_col(label), end_col = #label }
        end
        if self.adapter.item_extra_highlights then
          local extra = self.adapter.item_extra_highlights(item.key, entry, label)
          for _, hl in ipairs(extra or {}) do hl.line = #lines - 1; highlights[#highlights + 1] = hl end
        end
      end
    end
    if #lines == 0 then
      if self.snapshot.loading then
        lines = { self.snapshot.status ~= "" and self.snapshot.status or "Loading..." }
      elseif self.snapshot.last_err and self.snapshot.last_err ~= "" then
        lines = { self.snapshot.status ~= "" and self.snapshot.status or "Error", self.snapshot.last_err }
      elseif self.snapshot.status and self.snapshot.status ~= "" then
        lines = { self.snapshot.status }
      else
        lines = { "" }
      end
    end

    vim.bo[pane.buf].modifiable = true
    vim.api.nvim_buf_set_lines(pane.buf, 0, -1, false, lines)
    vim.api.nvim_buf_clear_namespace(pane.buf, vim.api.nvim_create_namespace("aztools-pane-" .. item.key), 0, -1)
    vim.bo[pane.buf].modifiable = false
    if item.key == "preview" then
      vim.bo[pane.buf].filetype = pane_filetype
      vim.bo[pane.buf].syntax = pane_filetype
      self:highlight_preview(pane.buf, pane_filetype, lines)
    end
    local pane_ns = vim.api.nvim_create_namespace("aztools-pane-" .. item.key)
    for _, hl in ipairs(highlights) do
      vim.api.nvim_buf_add_highlight(pane.buf, pane_ns, hl.group, hl.line, hl.start_col, hl.end_col)
    end
    for row, prefix in ipairs(prefixes) do
      if prefix and prefix.text and prefix.text ~= "" then
        vim.api.nvim_buf_set_extmark(pane.buf, pane_ns, row - 1, 0, {
          virt_text = { { prefix.text .. " ", prefix.hl or "Directory" } },
          virt_text_pos = "inline",
          right_gravity = false,
        })
      end
    end
    if self.adapter.item_virtual_lines then
      local row = 0
      for _, entry in ipairs(item.pane.visible_items) do
        local virt = self.adapter.item_virtual_lines(item.key, entry)
        if virt and #virt > 0 then
          vim.api.nvim_buf_set_extmark(pane.buf, pane_ns, row, 0, { virt_lines = { virt }, virt_lines_above = false })
        end
        row = row + 1
      end
    end

    if not pane._aztools_mapped then
      self:attach_keymaps(pane.buf)
      local buf_id = pane.buf
      vim.api.nvim_create_autocmd({ "CursorMoved", "BufEnter", "WinEnter" }, {
        buffer = buf_id,
        callback = function() self:on_cursor_moved(buf_id) end,
      })
      pane._aztools_mapped = true
    end

    local target = clamp(self.cursors[item.key] or 1, #lines)
    local target_col = item.key == "preview" and 0 or line_text_start_col(lines[target])
    vim.api.nvim_win_set_cursor(pane.win, { target, target_col })
  end

  for key, pane in pairs(self.ui.panes) do
    if not keep[key] then
      window.close(pane)
      self.ui.panes[key] = nil
    end
  end

  if force_focus then
    if target_key then
      self:ensure_focus(target_key)
    end
  elseif previous_focus then
    self:ensure_focus(previous_focus)
  end
  self.rendering = false
end

function Explorer:attach_keymaps(buf)
  local function map(lhs, rhs)
    if not lhs or lhs == false then return end
    vim.keymap.set("n", lhs, rhs, { buffer = buf, silent = true, nowait = true })
  end

  map("q", function() self:close() end)
  map("r", function() self:invoke(self.adapter.refresh_action(self)) end)
  map("j", function() self:move_cursor(1) end)
  map("k", function() self:move_cursor(-1) end)
  map("l", function() self:open_selected() end)
  map("<CR>", function() self:open_selected() end)
  map("h", function()
    if self.adapter.back_action then
      local action = self.adapter.back_action(self)
      if action then self:invoke(action); return end
    end
    self:invoke(self.adapter.left_action(self))
  end)
  if self.adapter.extra_keymaps then self.adapter.extra_keymaps(self, map) end
end

return Explorer

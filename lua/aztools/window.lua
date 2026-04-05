local M = {}

function M.origin()
  return { row = 0, col = 0 }
end

function M.get_first_valid_normal_window()
  for _, win_id in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
    if vim.api.nvim_win_get_config(win_id).relative == "" then
      return win_id
    end
  end
end

function M.focus(win_id)
  if not (type(win_id) == "number" and vim.api.nvim_win_is_valid(win_id)) then
    return
  end
  vim.api.nvim_set_current_win(win_id)
end

function M.set_buf(win_id, buf_id)
  if not (type(win_id) == "number" and vim.api.nvim_win_is_valid(win_id)) then
    return
  end
  vim.cmd(string.format("noautocmd call nvim_win_set_buf(%d, %d)", win_id, buf_id))
end

local function ensure_buf(existing, filetype, preview)
  if existing and existing.buf and vim.api.nvim_buf_is_valid(existing.buf) then
    if filetype and filetype ~= "" then
      vim.bo[existing.buf].filetype = filetype
      vim.bo[existing.buf].syntax = filetype
    end
    vim.bo[existing.buf].buftype = preview and "" or "nofile"
    return existing.buf
  end
  local buf = vim.api.nvim_create_buf(false, true)
  vim.bo[buf].bufhidden = "wipe"
  vim.bo[buf].buftype = preview and "" or "nofile"
  vim.bo[buf].swapfile = false
  vim.bo[buf].modifiable = false
  vim.bo[buf].filetype = filetype or "aztools"
  vim.bo[buf].syntax = filetype or "aztools"
  vim.b[buf].aztools_owned = true
  return buf
end

function M.ensure_pane(existing, rect, opts)
  opts = opts or {}
  local buf = ensure_buf(existing, opts.filetype, opts.preview)
  local config = {
    relative = "editor",
    row = rect.row,
    col = rect.col,
    width = rect.width,
    height = rect.height,
    style = "minimal",
    border = opts.border or "single",
    title = opts.title,
    title_pos = opts.title_pos or "left",
    focusable = true,
    zindex = 99,
  }

  local win
  if existing and existing.win and vim.api.nvim_win_is_valid(existing.win) then
    win = existing.win
    vim.api.nvim_win_set_config(win, config)
    M.set_buf(win, buf)
  else
    win = vim.api.nvim_open_win(buf, false, config)
  end

  vim.wo[win].number = false
  vim.wo[win].relativenumber = false
  vim.wo[win].signcolumn = "no"
  vim.wo[win].wrap = false
  vim.wo[win].cursorline = true
  vim.wo[win].winfixwidth = true
  vim.wo[win].winfixheight = true
  vim.wo[win].winhl = opts.winhl or "Normal:NormalFloat,FloatBorder:FloatBorder"
  if opts.pane_key and opts.pane_key ~= "" then
    pcall(vim.api.nvim_win_set_var, win, "aztools_pane_key", opts.pane_key)
  end

  return { buf = buf, win = win }
end

function M.close(state)
  if not state then
    return
  end
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    vim.api.nvim_win_close(state.win, true)
  end
end

return M

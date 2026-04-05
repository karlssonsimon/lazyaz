local M = {}
M.__index = M

M.spinner_presets = {
  default = { "⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷" },
}

function M.new()
  local self = setmetatable({}, M)
  self.spinner_symbols = M.spinner_presets.default
  self.spinner_index = 1
  self.spinner_timer = nil
  self.notify_id = nil
  return self
end

function M:update_spinner(pending_text)
  if not self.spinner_timer then
    return
  end
  self.notify_id = vim.notify(pending_text .. " " .. self.spinner_symbols[self.spinner_index], vim.log.levels.INFO, {
    title = "Aztools",
    id = "aztools-progress",
    replace = self.notify_id,
  })
  self.spinner_index = (self.spinner_index % #self.spinner_symbols) + 1
end

function M:start_spinner(pending_text)
  if self.spinner_timer then
    self:stop_spinner("", vim.log.levels.INFO)
  end
  self.spinner_timer = vim.loop.new_timer()
  self.spinner_timer:start(0, 120, vim.schedule_wrap(function()
    self:update_spinner(pending_text)
  end))
end

function M:stop_spinner(finish_text, level)
  level = level or vim.log.levels.INFO
  if self.spinner_timer then
    self.spinner_timer:stop()
    self.spinner_timer:close()
    self.spinner_timer = nil
  end
  if finish_text and finish_text ~= "" then
    vim.notify(finish_text, level, {
      title = "Aztools",
      id = "aztools-progress",
      replace = self.notify_id,
    })
  end
  self.notify_id = nil
end

return M

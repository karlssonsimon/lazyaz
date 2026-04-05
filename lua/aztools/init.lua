local config = require("aztools.config")
local install = require("aztools.install")
local process = require("aztools.process")

local M = {}

function M.setup(opts)
  local state = config.setup(opts)
  if state.runtime and state.runtime.auto_start then
    vim.schedule(function()
      process.autostart()
    end)
  end
  return state
end

M.blob = require("aztools.blob")
M.kv = require("aztools.kv")
M.sb = require("aztools.sb")

function M.build(module_name)
  if module_name and module_name ~= "" then
    return install.build(module_name)
  end
  return install.build_all()
end

function M.autostart()
  process.autostart()
end

return M

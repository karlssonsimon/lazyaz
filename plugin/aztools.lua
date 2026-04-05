if vim.g.loaded_aztools then
  return
end
vim.g.loaded_aztools = true

local function set_highlights()
  vim.api.nvim_set_hl(0, "AztoolsPane", { link = "NormalFloat", default = true })
  vim.api.nvim_set_hl(0, "AztoolsBorder", { link = "FloatBorder", default = true })
  vim.api.nvim_set_hl(0, "AztoolsTitle", { link = "Directory", default = true })
  vim.api.nvim_set_hl(0, "AztoolsPaneActive", { link = "NormalFloat", default = true })
  vim.api.nvim_set_hl(0, "AztoolsBorderActive", { link = "DiagnosticInfo", default = true })
  vim.api.nvim_set_hl(0, "AztoolsTitleActive", { link = "Title", default = true })
  vim.api.nvim_set_hl(0, "AztoolsPreviewTag", { link = "Keyword", default = true })
  vim.api.nvim_set_hl(0, "AztoolsPreviewString", { link = "String", default = true })
  vim.api.nvim_set_hl(0, "AztoolsPreviewKey", { link = "Constant", default = true })
  vim.api.nvim_set_hl(0, "AztoolsGhostText", { link = "Comment", default = true })
end

set_highlights()

vim.api.nvim_create_autocmd("ColorScheme", {
  group = vim.api.nvim_create_augroup("AztoolsHighlights", { clear = true }),
  callback = set_highlights,
})

vim.api.nvim_create_user_command("Azblob", function(opts)
  require("aztools.blob").command(opts.fargs[1], { line1 = opts.line1, line2 = opts.line2 })
end, {
  nargs = 1,
  range = true,
  complete = function()
    return { "open", "refresh", "download", "loadall", "yank", "put" }
  end,
})

vim.api.nvim_create_user_command("Azkv", function(opts)
  require("aztools.kv").command(opts.fargs[1])
end, {
  nargs = 1,
  complete = function()
    return { "open", "refresh" }
  end,
})

vim.api.nvim_create_user_command("Azsb", function(opts)
  require("aztools.sb").command(opts.fargs[1], { line1 = opts.line1, line2 = opts.line2 })
end, {
  nargs = 1,
  range = true,
  complete = function()
    return { "open", "refresh", "active", "dlq", "filter", "requeue", "delete" }
  end,
})
vim.api.nvim_create_user_command("AztoolsBuild", function(opts)
  local ok, result = pcall(require("aztools").build, opts.args ~= "" and opts.args or nil)
  if not ok then
    vim.notify(result, vim.log.levels.ERROR)
    return
  end
  if type(result) == "table" then
    local parts = {}
    for name, path in pairs(result) do
      table.insert(parts, string.format("%s=%s", name, path))
    end
    table.sort(parts)
    vim.notify("Built aztools daemons: " .. table.concat(parts, ", "))
    return
  end
  vim.notify("Built aztools daemon: " .. result)
end, {
  nargs = "?",
  complete = function() return { "blob", "kv", "sb" } end,
})

vim.api.nvim_create_user_command("AztoolsStart", function()
  require("aztools").autostart()
end, {})

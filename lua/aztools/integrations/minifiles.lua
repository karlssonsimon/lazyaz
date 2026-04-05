local M = {}

local function get_minifiles()
  local ok, mod = pcall(require, "mini.files")
  if not ok then
    return nil, "mini.files is not installed"
  end
  return mod
end

function M.resolve_target_dir()
  if vim.bo.filetype ~= "minifiles" then
    return nil, "Azblob put only works in mini.files"
  end

  local MiniFiles, err = get_minifiles()
  if not MiniFiles then
    return nil, err
  end

  local state = MiniFiles.get_explorer_state()
  if type(state) ~= "table" or type(state.branch) ~= "table" then
    return nil, "No active mini.files explorer"
  end

  local path = state.branch[state.depth_focus]
  if type(path) ~= "string" or path == "" then
    return nil, "No active mini.files target"
  end

  if vim.fn.isdirectory(path) == 1 then
    return path, MiniFiles
  end

  local parent = vim.fs.dirname(path)
  if type(parent) ~= "string" or parent == "" then
    return nil, "Could not resolve mini.files target directory"
  end

  return parent, MiniFiles
end

function M.refresh(mod)
  local MiniFiles = mod
  if not MiniFiles then
    MiniFiles = select(1, get_minifiles())
  end
  if not MiniFiles then
    return false
  end
  local ok = pcall(MiniFiles.refresh)
  return ok
end

return M

local M = {}

local state = {
  blob_clipboard = nil,
}

local function cleanup_dir(path)
  if type(path) ~= "string" or path == "" then
    return
  end
  pcall(vim.fn.delete, path, "rf")
end

function M.get_blob()
  if not state.blob_clipboard then
    return nil
  end
  return vim.deepcopy(state.blob_clipboard)
end

function M.set_blob(payload)
  if state.blob_clipboard and state.blob_clipboard.staging_root ~= payload.staging_root then
    cleanup_dir(state.blob_clipboard.staging_root)
  end
  state.blob_clipboard = vim.deepcopy(payload)
end

function M.clear_blob()
  if state.blob_clipboard then
    cleanup_dir(state.blob_clipboard.staging_root)
  end
  state.blob_clipboard = nil
end

return M

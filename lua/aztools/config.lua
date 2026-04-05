local M = {}

local function expand_path(path)
  if type(path) ~= "string" or path == "" then
    return path
  end
  if path:sub(1, 1) == "~" then
    return vim.fn.expand(path)
  end
  return path
end

local function normalize(cfg)
  cfg.install.repo_root = expand_path(cfg.install.repo_root)
  cfg.install.source_dir = expand_path(cfg.install.source_dir)
  cfg.install.binary_dir = expand_path(cfg.install.binary_dir)
  cfg.runtime.socket_dir = expand_path(cfg.runtime.socket_dir)
  cfg.runtime.cache_root = expand_path(cfg.runtime.cache_root)
  for _, name in ipairs({ "blob", "kv", "sb" }) do
    local mod = cfg[name]
    mod.binary_path = expand_path(mod.binary_path)
    mod.cache_db = expand_path(mod.cache_db)
    if mod.download_root then
      mod.download_root = expand_path(mod.download_root)
    end
  end
  return cfg
end

local defaults = {
  install = {
    repo = "karlssonsimon/azure-storage-tui",
    binary_dir = vim.fn.stdpath("data") .. "/aztools/bin",
    repo_root = nil,
    source_dir = nil,
    allow_build_from_source = true,
    prefer_source_build = true,
  },
  runtime = {
    socket_dir = vim.fn.stdpath("run") .. "/aztools",
    cache_root = vim.fn.stdpath("data") .. "/aztools",
    auto_start = false,
  },
  rpc = {
    timeout_ms = 60000,
  },
  blob = {
    enabled = true,
    width_focus = 42,
    width_nofocus = 22,
    width_preview = 60,
    max_height = 28,
    binary_name = "azblobd",
    binary_path = nil,
    cache_db = nil,
    download_root = vim.fn.stdpath("data") .. "/aztools/downloads",
  },
  kv = {
    enabled = true,
    width_focus = 42,
    width_nofocus = 22,
    width_preview = 60,
    max_height = 28,
    binary_name = "azkvd",
    binary_path = nil,
    cache_db = nil,
  },
  sb = {
    enabled = true,
    width_focus = 42,
    width_nofocus = 22,
    width_preview = 60,
    max_height = 28,
    binary_name = "azsbd",
    binary_path = nil,
    cache_db = nil,
  },
}

local state = vim.deepcopy(defaults)

function M.setup(opts)
  state = normalize(vim.tbl_deep_extend("force", vim.deepcopy(defaults), opts or {}))
  return state
end

function M.get()
  return state
end

function M.module(name)
  return state[name]
end

function M.cache_db(module_name)
  local module_cfg = state[module_name] or {}
  if module_cfg.cache_db and module_cfg.cache_db ~= "" then
    return module_cfg.cache_db
  end
  return string.format("%s/%s/cache.db", state.runtime.cache_root, module_name)
end

return M

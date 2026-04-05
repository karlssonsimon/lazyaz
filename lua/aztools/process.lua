local config = require("aztools.config")
local install = require("aztools.install")

local M = {}
local started = {}

local function supports_cache_db(binary)
  local result = vim.system({ binary, "--help" }, { text = true }):wait()
  local output = (result.stdout or "") .. "\n" .. (result.stderr or "")
  return output:find("%-%-cache%-db") ~= nil
end

local function source_is_newer(binary)
  if not binary or vim.loop.fs_stat(binary) == nil then
    return true
  end
  local binary_time = vim.fn.getftime(binary)
  local root = install.repo_root()
  local files = vim.fn.globpath(root, "**/*.go", false, true)
  for _, path in ipairs(files) do
    if vim.fn.getftime(path) > binary_time then
      return true
    end
  end
  return false
end

local function ensure_dir(path)
  vim.fn.mkdir(path, "p")
end

local function socket_path(name)
  local dir = config.get().runtime.socket_dir
  ensure_dir(dir)
  return string.format("%s/%s.sock", dir, name)
end

local function is_running(proc)
  return proc and proc.socket and vim.loop.fs_stat(proc.socket) ~= nil
end

function M.start(module_name, opts)
  opts = opts or {}
  local existing = started[module_name]
  if is_running(existing) then
    if opts.keepalive then
      existing.keepalive = true
    end
    return existing
  end

  local install_cfg = config.get().install or {}
  local binary, err

  if install_cfg.prefer_source_build then
    binary, err = install.resolve_binary(module_name)
    if not binary or not supports_cache_db(binary) or source_is_newer(binary) then
      binary = install.build(module_name)
    end
  else
    binary, err = install.resolve_binary(module_name)
    if not binary then
      binary = install.install(module_name)
    end
  end

  local sock = socket_path("aztools-" .. module_name)
  local cache_db = config.cache_db(module_name)
  ensure_dir(vim.fs.dirname(cache_db))
  vim.fn.delete(sock)

  local stderr_chunks, stdout_chunks = {}, {}
  local handle = vim.system(
    { binary, "--socket", sock, "--cache-db", cache_db },
    { detach = false, text = true },
    function(result)
      if result and result.stderr and result.stderr ~= "" then
        table.insert(stderr_chunks, result.stderr)
      end
      if result and result.stdout and result.stdout ~= "" then
        table.insert(stdout_chunks, result.stdout)
      end
    end
  )
  local ok = vim.wait(5000, function()
    return vim.loop.fs_stat(sock) ~= nil
  end, 50)
  if not ok then
    local stderr = table.concat(stderr_chunks, "")
    local stdout = table.concat(stdout_chunks, "")
    local detail = vim.trim(stderr ~= "" and stderr or stdout)
    if detail ~= "" then
      error(string.format("timed out waiting for %s socket: %s", module_name, detail))
    end
    error(string.format("timed out waiting for %s socket", module_name))
  end

  local proc = {
    module = module_name,
    binary = binary,
    socket = sock,
    cache_db = cache_db,
    handle = handle,
    session = nil,
    keepalive = opts.keepalive == true,
  }
  started[module_name] = proc
  return proc
end

function M.stop(proc)
  if not proc then
    return
  end
  if proc.keepalive then
    return
  end
  if proc.handle then
    pcall(proc.handle.kill, proc.handle, 15)
  end
  if proc.socket then
    vim.fn.delete(proc.socket)
  end
  if started[proc.module] == proc then
    started[proc.module] = nil
  end
end

function M.autostart()
  for _, name in ipairs({ "blob", "kv", "sb" }) do
    local module_cfg = config.module(name) or {}
    if module_cfg.enabled ~= false then
      pcall(M.start, name, { keepalive = true })
    end
  end
end

return M

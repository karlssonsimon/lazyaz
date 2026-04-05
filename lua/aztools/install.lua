local config = require("aztools.config")

local M = {}

local function dirname(path)
  return vim.fs.dirname(path)
end

local function default_repo_root()
  local source = debug.getinfo(1, "S").source:sub(2)
  return dirname(dirname(dirname(source)))
end

local function uname()
  local raw = vim.loop.os_uname()
  local os_map = { Darwin = "darwin", Linux = "linux", Windows_NT = "windows" }
  local arch_map = { x86_64 = "amd64", arm64 = "arm64", aarch64 = "arm64" }
  return os_map[raw.sysname] or raw.sysname:lower(), arch_map[raw.machine] or raw.machine:lower()
end

local function file_exists(path)
  return path and vim.loop.fs_stat(path) ~= nil
end

local function ensure_dir(path)
  vim.fn.mkdir(path, "p")
end

local function repo_root()
  local install_cfg = config.get().install
  return install_cfg.source_dir or install_cfg.repo_root or default_repo_root()
end

function M.repo_root()
  return repo_root()
end

local function binary_path(module_name)
  local cfg = config.module(module_name)
  if cfg.binary_path and cfg.binary_path ~= "" then
    return cfg.binary_path
  end
  local install_cfg = config.get().install
  return install_cfg.binary_dir .. "/" .. cfg.binary_name
end

function M.binary_path(module_name)
  return binary_path(module_name)
end

function M.resolve_binary(module_name)
  local module_cfg = config.module(module_name)
  local path = binary_path(module_name)
  if file_exists(path) then
    return path
  end
  local repo_binary = repo_root() .. "/" .. module_cfg.binary_name
  if file_exists(repo_binary) then
    return repo_binary
  end
  return nil, string.format(
    "binary for %s not found at %s. Set %s.binary_path or install a release binary.",
    module_name,
    path,
    module_name
  )
end

local function build_from_source(module_name)
  local install_cfg = config.get().install
  if not install_cfg.allow_build_from_source then
    return nil, "source build fallback disabled"
  end

  local module_cfg = config.module(module_name)
  local target_dir = install_cfg.binary_dir
  local target = target_dir .. "/" .. module_cfg.binary_name
  ensure_dir(target_dir)

  local cmd_pkg = "./cmd/" .. module_cfg.binary_name
  local result = vim.system({ "go", "build", "-o", target, cmd_pkg }, { text = true, cwd = repo_root() }):wait()
  if result.code ~= 0 then
    local stderr = vim.trim((result.stderr or "") ~= "" and result.stderr or (result.stdout or ""))
    return nil, string.format("failed to build %s from source: %s", module_cfg.binary_name, stderr)
  end
  if not file_exists(target) then
    return nil, string.format("built %s but binary was not found at %s", module_cfg.binary_name, target)
  end
  vim.fn.setfperm(target, "rwxr-xr-x")
  return target
end

function M.install(module_name)
  local install_cfg = config.get().install
  local module_cfg = config.module(module_name)
  if install_cfg.prefer_source_build then
    local built, build_err = build_from_source(module_name)
    if built then
      return built
    end
    error(build_err)
  end

  local os_name, arch = uname()
  local target_dir = install_cfg.binary_dir
  ensure_dir(target_dir)
  local api = string.format("https://api.github.com/repos/%s/releases/latest", install_cfg.repo)
  local release = vim.system({ "curl", "-fsSL", api }, { text = true }):wait()
  if release.code ~= 0 then
    local built, build_err = build_from_source(module_name)
    if built then
      return built
    end
    error("failed to fetch release metadata; " .. build_err)
  end

  local decoded = vim.json.decode(release.stdout)
  local wanted = nil
  local daemon = module_cfg.binary_name:lower()
  for _, asset in ipairs(decoded.assets or {}) do
    local name = string.lower(asset.name)
    if name:find(daemon, 1, true) and name:find(os_name, 1, true) and name:find(arch, 1, true) then
      wanted = asset
      break
    end
  end
  if not wanted then
    local built, build_err = build_from_source(module_name)
    if built then
      return built
    end
    error(string.format("no release asset found for %s (%s/%s); %s", daemon, os_name, arch, build_err))
  end

  local archive = target_dir .. "/" .. wanted.name
  local download = vim.system({ "curl", "-fsSL", "-o", archive, wanted.browser_download_url }, { text = true }):wait()
  if download.code ~= 0 then
    local built, build_err = build_from_source(module_name)
    if built then
      return built
    end
    error("failed to download release asset; " .. build_err)
  end

  if archive:sub(-7) == ".tar.gz" then
    local extract = vim.system({ "tar", "-xzf", archive, "-C", target_dir }, { text = true }):wait()
    if extract.code ~= 0 then
      local built, build_err = build_from_source(module_name)
      if built then
        return built
      end
      error("failed to extract release archive; " .. build_err)
    end
  elseif archive:sub(-4) == ".zip" then
    local extract = vim.system({ "unzip", "-o", archive, "-d", target_dir }, { text = true }):wait()
    if extract.code ~= 0 then
      local built, build_err = build_from_source(module_name)
      if built then
        return built
      end
      error("failed to extract release zip; " .. build_err)
    end
  end

  local target = binary_path(module_name)
  if not file_exists(target) then
    error(string.format("expected binary %s after extraction", target))
  end
  vim.fn.setfperm(target, "rwxr-xr-x")
  return target
end

function M.build(module_name)
  local built, err = build_from_source(module_name)
  if built then
    return built
  end
  error(err)
end

function M.build_all()
  local results = {}
  for _, name in ipairs({ "blob", "kv", "sb" }) do
    results[name] = M.build(name)
  end
  return results
end

return M

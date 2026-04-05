local M = {}

local function timeout_ms()
  local ok, config = pcall(require, "aztools.config")
  if ok and config and config.get then
    local rpc = config.get().rpc or {}
    return rpc.timeout_ms or 60000
  end
  return 60000
end

local function next_id()
  return tostring(vim.loop.hrtime())
end

local function call_once(socket, session, method, params)
  local pipe = vim.loop.new_pipe(false)
  local done = false
  local response = nil
  local err_msg = nil
  local chunks = {}

  pipe:connect(socket, function(err)
    if err then
      err_msg = err
      done = true
      return
    end
    pipe:read_start(function(read_err, data)
      if read_err then
        err_msg = read_err
        done = true
        return
      end
      if data then
        table.insert(chunks, data)
        local joined = table.concat(chunks)
        local newline = joined:find("\n", 1, true)
        if newline then
          local payload = joined:sub(1, newline - 1)
          response = vim.json.decode(payload)
          done = true
        end
      end
    end)
    local payload = vim.json.encode({
      id = next_id(),
      session = session,
      method = method,
      params = params,
    }) .. "\n"
    pipe:write(payload)
  end)

  local ok = vim.wait(timeout_ms(), function()
    return done
  end, 20)
  pipe:shutdown()
  pipe:close()
  if not ok then
    error("rpc call timed out")
  end
  if err_msg then
    error(err_msg)
  end
  if not response.ok then
    error(response.error or "rpc request failed")
  end
  return response.result
end

function M.create_session(socket)
  return call_once(socket, nil, "session.create", nil)
end

function M.close_session(socket, session)
  if not session or session == "" then
    return nil
  end
  return call_once(socket, session, "session.close", nil)
end

function M.state(socket, session)
  return call_once(socket, session, "state.get", nil)
end

function M.action(socket, session, params)
  return call_once(socket, session, "action.invoke", params)
end

return M

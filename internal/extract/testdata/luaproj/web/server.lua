local socket = require("socket")

local Server = {}

function Server.start(self)
  boot()
  return socket
end

function boot()
  return add(1, 2)
end

return Server

-- math utilities

function add(a, b)
  return a + b
end

local M = {}

function M.double(x)
  return add(x, x)
end

return M

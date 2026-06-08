using LinearAlgebra
import Printf

struct Circle
    radius::Float64
end

module Shapes

function area(c)
    return scale(c)
end

function scale(c)
    return square(c)
end

end # module Shapes

const std = @import("std");

pub fn add(a: i32, b: i32) i32 {
    return a + b;
}

pub fn double(x: i32) i32 {
    return add(x, x);
}

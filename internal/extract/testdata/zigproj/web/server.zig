const std = @import("std");

pub const Server = struct {
    port: u16,

    pub fn start(self: *Server) void {
        _ = self;
        boot();
    }
};

pub fn boot() void {
    const total = add(1, 2);
    _ = total;
}

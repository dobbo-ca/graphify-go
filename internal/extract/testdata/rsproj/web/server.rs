use crate::util::math::add;

pub struct Server {
    pub port: u16,
}

impl Server {
    pub fn start(&self) -> i32 {
        boot()
    }
}

fn boot() -> i32 {
    add(1, 2)
}

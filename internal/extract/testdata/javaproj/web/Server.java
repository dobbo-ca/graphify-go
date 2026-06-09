package web;

import util.Math;

public class Server {
    private int port;

    public int start() {
        return boot();
    }

    private int boot() {
        return Math.add(1, 2);
    }
}

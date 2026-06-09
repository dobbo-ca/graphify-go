package web

import kotlin.math.sqrt

class Server(val port: Int) {
    fun start(): Int {
        return boot()
    }

    fun boot(): Int {
        return add(1, 2)
    }
}

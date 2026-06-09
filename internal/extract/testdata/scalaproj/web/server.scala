package web

import scala.collection.mutable.Map

def boot(): Int = add(1, 2)

class Server {
  def start(): Int = boot()
}

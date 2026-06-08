<?php

use Psr\Log\LoggerInterface;

class Server
{
    public function start()
    {
        return boot();
    }
}

function boot()
{
    return add(1, 2);
}

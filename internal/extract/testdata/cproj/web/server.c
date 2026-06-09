#include <stdio.h>
#include "helper.h"

struct Server {
    int port;
    int (*start)(struct Server *s);
};

int boot(int n) {
    return add(n, 1);
}

int run(void) {
    return boot(8080);
}

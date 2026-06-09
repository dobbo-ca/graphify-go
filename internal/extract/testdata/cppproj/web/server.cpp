#include <string>

int boot();

class Server {
public:
    int start() {
        return boot();
    }
};

int boot() {
    return add(1, 2);
}

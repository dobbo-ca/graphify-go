using System.Text;

int Boot()
{
    return Add(1, 2);
}

namespace Web
{
    class Server
    {
        public int Start()
        {
            return Boot();
        }
    }
}

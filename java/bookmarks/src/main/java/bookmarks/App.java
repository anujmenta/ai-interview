package bookmarks;

import com.sun.net.httpserver.HttpServer;
import java.net.InetSocketAddress;

public final class App {
    public static void main(String[] args) throws Exception {
        Db.init();

        int port = Integer.parseInt(System.getenv().getOrDefault("PORT", "8080"));
        HttpServer server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/", new Routes(new Store()));
        server.start();
        System.out.println("listening on http://localhost:" + port);
    }
}

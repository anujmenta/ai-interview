package bookmarks;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.sql.SQLException;
import java.util.Map;
import java.util.Optional;

public final class Routes implements HttpHandler {
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final String NOT_FOUND = "bookmark not found";

    private final Store store;

    public Routes(Store store) {
        this.store = store;
    }

    private record Input(String url, String title, String description) {}

    /** Thrown for client errors; carries the status and message returned to the caller. */
    private static final class HttpError extends Exception {
        final int status;

        HttpError(int status, String message) {
            super(message);
            this.status = status;
        }
    }

    @Override
    public void handle(HttpExchange ex) throws IOException {
        try {
            route(ex);
        } catch (HttpError e) {
            writeJson(ex, e.status, Map.of("error", e.getMessage()));
        } catch (SQLException e) {
            writeJson(ex, 500, Map.of("error", String.valueOf(e.getMessage())));
        } finally {
            ex.close();
        }
    }

    private void route(HttpExchange ex) throws IOException, SQLException, HttpError {
        String method = ex.getRequestMethod();
        String path = ex.getRequestURI().getPath();
        if (path.length() > 1 && path.endsWith("/")) {
            path = path.substring(0, path.length() - 1);
        }

        if (path.equals("/bookmarks")) {
            switch (method) {
                case "GET" -> writeJson(ex, 200, store.listBookmarks());
                case "POST" -> {
                    Input in = parseInput(ex);
                    writeJson(ex, 201, store.createBookmark(in.url(), in.title(), in.description()));
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        if (path.startsWith("/bookmarks/")) {
            long id = parseId(path.substring("/bookmarks/".length()));
            switch (method) {
                case "GET" -> writeJson(ex, 200, store.getBookmark(id).orElseThrow(() -> new HttpError(404, NOT_FOUND)));
                case "PUT" -> {
                    Input in = parseInput(ex);
                    Optional<Bookmark> updated = store.updateBookmark(id, in.url(), in.title(), in.description());
                    writeJson(ex, 200, updated.orElseThrow(() -> new HttpError(404, NOT_FOUND)));
                }
                case "DELETE" -> {
                    if (!store.deleteBookmark(id)) {
                        throw new HttpError(404, NOT_FOUND);
                    }
                    ex.sendResponseHeaders(204, -1);
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        throw new HttpError(404, "not found");
    }

    private static long parseId(String raw) throws HttpError {
        try {
            return Long.parseLong(raw);
        } catch (NumberFormatException e) {
            throw new HttpError(404, NOT_FOUND);
        }
    }

    private static Input parseInput(HttpExchange ex) throws IOException, HttpError {
        JsonNode data;
        try (InputStream body = ex.getRequestBody()) {
            data = JSON.readTree(body);
        } catch (IOException e) {
            throw new HttpError(400, "body must be a JSON object");
        }
        if (data == null || !data.isObject()) {
            throw new HttpError(400, "body must be a JSON object");
        }
        String url = requiredString(data, "url");
        String title = requiredString(data, "title");
        JsonNode desc = data.get("description");
        if (desc != null && !desc.isTextual()) {
            throw new HttpError(400, "description must be a string");
        }
        return new Input(url, title, desc == null ? "" : desc.asText());
    }

    private static String requiredString(JsonNode data, String field) throws HttpError {
        JsonNode node = data.get(field);
        if (node == null || !node.isTextual() || node.asText().isBlank()) {
            throw new HttpError(400, field + " is required");
        }
        return node.asText().strip();
    }

    private static void writeJson(HttpExchange ex, int status, Object value) throws IOException {
        byte[] bytes = JSON.writeValueAsString(value).getBytes(StandardCharsets.UTF_8);
        ex.getResponseHeaders().set("Content-Type", "application/json");
        ex.sendResponseHeaders(status, bytes.length);
        try (OutputStream out = ex.getResponseBody()) {
            out.write(bytes);
        }
    }
}

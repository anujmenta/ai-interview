package bookmarks;

import java.util.List;

public record MoveResult(MoveResult.Status status, List<Bookmark> bookmarks) {
    public enum Status { OK, NOT_MEMBER, OUT_OF_RANGE }
}

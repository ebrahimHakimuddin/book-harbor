package dev.bookharbor.app.sync

import dev.bookharbor.app.reader.Annotation
import dev.bookharbor.app.reader.AnnotationKind
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Test

class AnnotationCodecTest {
    @Test
    fun requestMatchesTheServerShapeAndResponsesDecodeBack() {
        val highlight = Annotation(
            bookId = "book_1", kind = AnnotationKind.Highlight, locator = Locator.epub("epubcfi(/6/4!/4/2:5)"), label = "Ch 2",
            excerpt = "passage", note = "note", createdAt = "2026-09-20T10:00:00Z", syncId = "ann_1", endOffset = 12,
            updatedAt = "2026-09-21T10:00:00Z", deleted = true,
        )
        val change = JSONObject(AnnotationCodec.encodeRequest(7, listOf(highlight))).getJSONArray("changes").getJSONObject(0)
        assertEquals("highlight", change.getString("kind"))
        assertEquals("ann_1", change.getString("id"))

        val decoded = AnnotationCodec.decodeResponse(
            """{"cursor":9,"hasMore":false,"accepted":["ann_1"],"rejected":[],"annotations":[$change]}""",
        )
        assertEquals(9L, decoded.cursor)
        assertEquals(setOf("ann_1"), decoded.accepted)
        assertEquals(highlight, decoded.annotations.single())
    }
}

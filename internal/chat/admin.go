package chat

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/i18n"
)

// The moderation half of the room.
//
// Until now the platform had no way to look at what was being said in a shared
// space, and no way to remove anything from it. An admin could ban the person —
// which leaves the message up — or edit the database by hand.

// SetAudit wires the recorder. Optional, like everywhere else: without it the
// buttons still work and nothing records who pressed them, which is a
// deployment mistake rather than a code path.
func (m *Module) SetAudit(a *audit.Recorder) { m.audit = a }

// AdminRecent lists the newest messages in the room, deleted ones included.
//
// Not audited. Reading a shared room is not reading somebody's private work: it
// is the same text every signed-in student can already scroll to. Direct
// messages between two people are a different matter and are not in this list —
// see the peer filter below.
func (m *Module) AdminRecent(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	msgs, err := m.repo.Recent(c.Request.Context(), limit)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": i18n.Msg(c, "lỗi máy chủ")})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

// AdminDelete removes a message whoever wrote it.
//
// Audited: this is an action taken on somebody else's words, which is exactly
// the class of thing the audit log exists for. Idempotent — a message already
// taken back answers the same way, because "it is gone" is what the caller
// asked for.
func (m *Module) AdminDelete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": i18n.Msg(c, "id không hợp lệ")})
		return
	}
	msg, err := m.repo.DeleteAsAdmin(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound,
			gin.H{"error": i18n.Msg(c, "tin nhắn không tồn tại")})
		return
	}
	if m.audit != nil {
		m.audit.Record(c, audit.Entry{
			ActorID:    m.userID(c),
			Action:     audit.ActionChatDelete,
			TargetType: audit.TargetChatMessage,
			TargetID:   strconv.FormatInt(id, 10),
			TargetName: msg.Username,
		})
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

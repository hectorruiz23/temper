package dev.temper.model

sealed interface SessionOpening {
    data class Continue(val sessionId: String, val revision: Long) : SessionOpening
    data object Create : SessionOpening
}

fun decideSessionOpening(
    performed: List<PerformedSession>,
    date: String,
    templateId: String,
): SessionOpening {
    val match = performed.firstOrNull { session ->
        session.date == date &&
            !session.id.isNullOrBlank() &&
            (templateId.isBlank() || session.templateId == templateId)
    }
    val sessionId = match?.id
    return if (match != null && sessionId != null) {
        SessionOpening.Continue(sessionId, match.revision)
    } else {
        SessionOpening.Create
    }
}

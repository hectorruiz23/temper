package dev.temper.ui.sets

import dev.temper.model.PlanSession

object SessionTemplateLookup {
    fun inPlan(sessions: List<PlanSession>, templateId: String): PlanSession? =
        sessions.firstOrNull { it.id == templateId }

    fun missingMessage(templateId: String, date: String): String =
        "This session was planned from a plan version that is not the one in force, " +
            "and the plan for $date does not list \"$templateId\" either. " +
            "Nothing can be logged against it: discard the session and start it again from the week."
}

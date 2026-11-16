package dev.temper.ui

import java.math.BigDecimal
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.time.temporal.WeekFields
import java.util.Locale

private val dayFormatter = DateTimeFormatter.ofPattern("EEE d", Locale.ENGLISH)
private val longDayFormatter = DateTimeFormatter.ofPattern("EEE d MMM", Locale.ENGLISH)
private val monthFormatter = DateTimeFormatter.ofPattern("MMM", Locale.ENGLISH)

fun parseDate(value: String?): LocalDate? = value?.let { runCatching { LocalDate.parse(it) }.getOrNull() }

fun dayLabel(date: LocalDate): String = date.format(dayFormatter)

fun longDayLabel(date: LocalDate): String = date.format(longDayFormatter)

fun weekLabel(start: String?, end: String?): String {
    val startDate = parseDate(start) ?: return "This week"
    val endDate = parseDate(end) ?: startDate
    val week = startDate.get(WeekFields.ISO.weekOfWeekBasedYear())
    return if (startDate.month == endDate.month) {
        "${startDate.dayOfMonth}–${endDate.dayOfMonth} ${endDate.format(monthFormatter)} ${endDate.year} · W$week"
    } else {
        "${startDate.dayOfMonth} ${startDate.format(monthFormatter)} – ${endDate.dayOfMonth} ${endDate.format(monthFormatter)} ${endDate.year} · W$week"
    }
}

fun formatKg(value: Double?): String {
    if (value == null) return "—"
    val decimal = BigDecimal.valueOf(value).stripTrailingZeros()
    return decimal.toPlainString()
}

fun formatSet(kg: Double, reps: Int): String = "${formatKg(kg)}×$reps"

fun formatSleep(milliseconds: Long?): String {
    if (milliseconds == null) return "—"
    val totalMinutes = milliseconds / 60_000
    val hours = totalMinutes / 60
    val minutes = totalMinutes % 60
    return "${hours} h ${minutes} m"
}

fun formatTarget(exercise: dev.temper.model.PlanExercise): String {
    val sets = exercise.targetSets
    val reps = when {
        exercise.repsMin != null && exercise.repsMax != null && exercise.repsMin != exercise.repsMax ->
            "${exercise.repsMin}–${exercise.repsMax}"
        exercise.repsMin != null -> exercise.repsMin.toString()
        exercise.repsMax != null -> exercise.repsMax.toString()
        else -> "?"
    }
    val base = if (sets != null) "$sets×$reps" else "$reps reps"
    val kg = exercise.targetKg?.let { " · ${formatKg(it)} kg" } ?: ""
    val tempo = exercise.tempo?.let { " · tempo $it" } ?: ""
    return base + kg + tempo
}

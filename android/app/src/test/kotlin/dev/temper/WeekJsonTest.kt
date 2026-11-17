package dev.temper

import dev.temper.model.WeekResponse
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class WeekJsonTest {
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        encodeDefaults = false
    }

    @Test
    fun mapsTheWeekResponseAndKeepsUnavailableMetricsDistinct() {
        val body = """
            {
              "week_start":"2026-11-16",
              "week_end":"2026-11-22",
              "planned":[
                {"date":"2026-11-19","template_id":"upper-a","discipline":"strength","target":"Upper body A","plan_version_id":4,"server_note":"ignored"}
              ],
              "performed":[],
              "days":[
                {"date":"2026-11-19","revision":3,"sleep_ms":25920000,"steps":6840,"resting_hr_bpm":52,"hrv_rmssd_ms":48.5},
                {"date":"2026-11-18","revision":2,"sleep_ms":0,"steps":null,"resting_hr_bpm":null,"hrv_rmssd_ms":null}
              ],
              "new_server_field":true
            }
        """.trimIndent()

        val week = json.decodeFromString<WeekResponse>(body)

        assertEquals("2026-11-16", week.weekStart)
        assertEquals(1, week.planned.size)
        assertEquals("upper-a", week.planned[0].templateId)
        assertEquals(4L, week.planned[0].planVersionId)
        assertEquals(2, week.days.size)
        assertEquals(25920000L, week.days[0].sleepMs)
        assertEquals(48.5, week.days[0].hrvRmssdMs!!, 0.0)
        assertEquals(0L, week.days[1].sleepMs)
        assertNull(week.days[1].steps)
        assertNull(week.days[1].restingHrBpm)
        assertTrue(week.performed.isEmpty())
    }

    @Test
    fun anEmptyWeekDecodesToEmptyLists() {
        val week = json.decodeFromString<WeekResponse>(
            """{"week_start":"2026-11-23","week_end":"2026-11-29","planned":[],"performed":[],"days":[]}""",
        )

        assertTrue(week.planned.isEmpty())
        assertTrue(week.days.isEmpty())
        assertTrue(week.performed.isEmpty())
    }

    @Test
    fun performedSessionsKeepTheirCompletenessAndEvidence() {
        val body = """
            {
              "week_start":"2026-11-16",
              "week_end":"2026-11-22",
              "planned":[],
              "performed":[
                {"id":"session-1","date":"2026-11-17","template_id":"s1","discipline":"strength",
                 "started_at":"2026-11-17T08:00:00Z","revision":2,"complete":true,
                 "evidence":{"source":"reported","reported_completed_at":"2026-11-17T09:00:00Z"},
                 "ignored":"new"},
                {"id":"session-2","date":"2026-11-18","template_id":"s2","discipline":"strength",
                 "started_at":"2026-11-18T08:00:00Z","revision":1,"complete":false,"evidence":null},
                {"id":"session-3","date":"2026-11-19","template_id":"s3","discipline":"run",
                 "started_at":"2026-11-19T08:00:00Z","revision":3,"complete":true,
                 "evidence":{"source":"watch","workout_id":"workout-1"}}
              ],
              "days":[]
            }
        """.trimIndent()

        val week = json.decodeFromString<WeekResponse>(body)

        assertEquals(3, week.performed.size)
        val complete = week.performed[0]
        assertEquals("session-1", complete.id)
        assertEquals("2026-11-17", complete.date)
        assertEquals("s1", complete.templateId)
        assertEquals("strength", complete.discipline)
        assertEquals("2026-11-17T08:00:00Z", complete.startedAt)
        assertEquals(2L, complete.revision)
        assertTrue(complete.complete)
        assertEquals("reported", complete.evidence?.source)
        assertEquals("2026-11-17T09:00:00Z", complete.evidence?.reportedCompletedAt)
        assertNull(complete.evidence?.workoutId)

        val open = week.performed[1]
        assertFalse(open.complete)
        assertNull(open.evidence)

        val watch = week.performed[2]
        assertTrue(watch.complete)
        assertEquals("watch", watch.evidence?.source)
        assertEquals("workout-1", watch.evidence?.workoutId)
        assertNull(watch.evidence?.reportedCompletedAt)
    }

    @Test
    fun aPayloadWithoutPerformedDecodesToAnEmptyList() {
        val week = json.decodeFromString<WeekResponse>(
            """{"week_start":"2026-11-23","week_end":"2026-11-29","planned":[],"days":[]}""",
        )

        assertTrue(week.performed.isEmpty())
    }
}

package dev.temper.runner

import dev.temper.clock.DistanceSource
import dev.temper.clock.MonotonicClock
import dev.temper.clock.WallClock
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class IntervalRunnerTest {
    @Test
    fun mixedTimedAndDistanceStepsTransitionExactlyAtTheirBoundaries() {
        val monotonic = FakeMonotonicClock()
        val distance = FakeDistanceSource(0.0)
        val runner = runner(
            steps = listOf(
                RunnerStep(0, "warm up", durationMs = 1_000),
                RunnerStep(1, "rep 1", distanceM = 10.0),
                RunnerStep(2, "recover", durationMs = 500),
            ),
            monotonic = monotonic,
            distance = distance,
        )
        runner.start()

        monotonic.now = 999
        var tick = runner.tick()
        assertEquals(0, tick.snapshot.stepIndex)
        assertEquals(1L, tick.snapshot.stepRemainingMs)
        assertTrue(tick.transitions.isEmpty())

        monotonic.now = 1_000
        tick = runner.tick()
        assertEquals(1, tick.snapshot.stepIndex)
        assertEquals(1, tick.transitions.size)
        assertEquals("rep 1", tick.transitions.single().label)
        assertTrue(tick.snapshot.currentStep.isDistance)

        distance.meters = 9.999
        monotonic.now = 1_001
        tick = runner.tick()
        assertEquals(1, tick.snapshot.stepIndex)
        assertFalse(tick.snapshot.finished)

        distance.meters = 10.0
        monotonic.now = 1_002
        tick = runner.tick()
        assertEquals(2, tick.snapshot.stepIndex)
        assertEquals(500L, tick.snapshot.stepRemainingMs)
        assertEquals(0L, tick.snapshot.stepElapsedMs)

        monotonic.now = 1_502
        tick = runner.tick()
        assertTrue(tick.snapshot.finished)
        assertEquals(RunnerStatus.Finished, tick.snapshot.status)
        assertTrue(tick.transitions.single().finished)
        assertEquals(1_502L, tick.snapshot.runElapsedMs)
    }

    @Test
    fun pauseStopsTheClockAndWallClockJumpsChangeNothing() {
        val monotonic = FakeMonotonicClock()
        val wall = FakeWallClock()
        val runner = runner(
            steps = listOf(
                RunnerStep(0, "first", durationMs = 1_000),
                RunnerStep(1, "second", durationMs = 1_000),
            ),
            monotonic = monotonic,
            wall = wall,
        )
        runner.start()

        monotonic.now = 400
        runner.tick()
        assertEquals(400L, runner.snapshot().stepElapsedMs)

        runner.pause()
        assertEquals(RunnerStatus.Paused, runner.snapshot().status)
        wall.now -= 3 * 60 * 60 * 1_000L
        monotonic.now = 10_000
        runner.tick()
        assertEquals(400L, runner.snapshot().stepElapsedMs)
        assertEquals(0, runner.snapshot().stepIndex)
        assertFalse(runner.snapshot().finished)

        runner.resume()
        wall.now += 6 * 60 * 60 * 1_000L
        monotonic.now = 10_600
        runner.tick()
        assertEquals(1, runner.snapshot().stepIndex)
        assertEquals(0L, runner.snapshot().stepElapsedMs)
        assertEquals(1_000L, runner.snapshot().runElapsedMs)

        monotonic.now = 11_600
        runner.tick()
        assertTrue(runner.snapshot().finished)
        assertEquals(2_000L, runner.snapshot().runElapsedMs)
    }

    @Test
    fun losingLocationSuspendsTheStepAndNeverFinishesItOnTimeAlone() {
        val monotonic = FakeMonotonicClock()
        val distance = FakeDistanceSource(0.0)
        val runner = runner(
            steps = listOf(
                RunnerStep(0, "rep 1", distanceM = 100.0),
                RunnerStep(1, "recover", durationMs = 1_000),
            ),
            monotonic = monotonic,
            distance = distance,
        )
        runner.start()

        monotonic.now = 1_000
        runner.tick()
        distance.meters = 40.0
        monotonic.now = 2_000
        runner.tick()
        assertEquals(40.0, runner.snapshot().stepDistanceDoneM, 0.001)

        distance.meters = null
        monotonic.now += 40 * 60 * 1_000L
        var tick = runner.tick()
        assertFalse(tick.snapshot.finished)
        assertEquals(0, tick.snapshot.stepIndex)
        assertFalse(tick.snapshot.distanceAvailable)
        assertEquals(40.0, tick.snapshot.stepDistanceDoneM, 0.001)

        distance.meters = 40.0
        monotonic.now += 500
        tick = runner.tick()
        assertFalse(tick.snapshot.finished)
        assertEquals(40.0, tick.snapshot.stepDistanceDoneM, 0.001)

        distance.meters = 140.0
        monotonic.now += 500
        tick = runner.tick()
        assertEquals(1, tick.snapshot.stepIndex)
        assertFalse(tick.snapshot.finished)

        monotonic.now += 1_000
        tick = runner.tick()
        assertTrue(tick.snapshot.finished)
    }

    @Test
    fun aCallbackFortyMinutesLateKeepsTheTimedSessionExact() {
        val monotonic = FakeMonotonicClock()
        val runner = runner(
            steps = listOf(
                RunnerStep(0, "first", durationMs = 600_000),
                RunnerStep(1, "second", durationMs = 300_000),
            ),
            monotonic = monotonic,
        )
        runner.start()

        monotonic.now = 40 * 60 * 1_000L
        val tick = runner.tick()

        assertTrue(tick.snapshot.finished)
        assertEquals(2, tick.transitions.size)
        assertEquals(900_000L, tick.snapshot.runElapsedMs)
    }

    @Test
    fun pausingADistanceStepFreezesProgressInsteadOfReplayingIt() {
        val monotonic = FakeMonotonicClock()
        val distance = FakeDistanceSource(0.0)
        val runner = runner(
            steps = listOf(RunnerStep(0, "rep 1", distanceM = 100.0)),
            monotonic = monotonic,
            distance = distance,
        )
        runner.start()

        monotonic.now = 1_000
        runner.tick()
        distance.meters = 20.0
        monotonic.now = 2_000
        runner.tick()
        assertEquals(20.0, runner.snapshot().stepDistanceDoneM, 0.001)

        runner.pause()
        distance.meters = 50.0
        monotonic.now = 10_000
        runner.tick()
        assertEquals(20.0, runner.snapshot().stepDistanceDoneM, 0.001)

        runner.resume()
        monotonic.now = 11_000
        runner.tick()
        assertEquals(20.0, runner.snapshot().stepDistanceDoneM, 0.001)

        distance.meters = 70.0
        monotonic.now = 12_000
        runner.tick()
        assertEquals(40.0, runner.snapshot().stepDistanceDoneM, 0.001)
    }

    @Test
    fun noiseBelowTheMinimumSampleIsIgnoredUntilItBecomesRealDistance() {
        val monotonic = FakeMonotonicClock()
        val distance = FakeDistanceSource(0.0)
        val runner = runner(
            steps = listOf(RunnerStep(0, "rep 1", distanceM = 100.0)),
            monotonic = monotonic,
            distance = distance,
        )
        runner.start()

        distance.meters = 0.5
        monotonic.now = 1_000
        runner.tick()
        assertEquals(0.0, runner.snapshot().stepDistanceDoneM, 0.001)

        distance.meters = 9.5
        monotonic.now = 2_000
        runner.tick()
        assertEquals(9.5, runner.snapshot().stepDistanceDoneM, 0.001)
    }

    @Test
    fun noNetworkCallsHappenBetweenTheFetchAndTheFinish() {
        val network = RecordingNetwork()
        network.fetchPlan()
        val callsAfterFetch = network.calls

        val monotonic = FakeMonotonicClock()
        val distance = FakeDistanceSource(0.0)
        val runner = runner(
            steps = listOf(
                RunnerStep(0, "warm up", durationMs = 1_000),
                RunnerStep(1, "rep", distanceM = 5.0),
            ),
            monotonic = monotonic,
            distance = distance,
        )
        runner.start()
        monotonic.now = 1_000
        runner.tick()
        distance.meters = 5.0
        monotonic.now = 1_500
        runner.tick()

        assertTrue(runner.snapshot().finished)
        assertEquals(callsAfterFetch, network.calls)
        assertEquals(1, network.calls)
    }

    private fun runner(
        steps: List<RunnerStep>,
        monotonic: FakeMonotonicClock = FakeMonotonicClock(),
        wall: FakeWallClock = FakeWallClock(),
        distance: FakeDistanceSource = FakeDistanceSource(null),
    ): IntervalRunner = IntervalRunner(steps, monotonic, wall, distance)

    private class FakeMonotonicClock(var now: Long = 0L) : MonotonicClock {
        override fun elapsedRealtimeMs(): Long = now
    }

    private class FakeWallClock(var now: Long = 1_700_000_000_000L) : WallClock {
        override fun nowMs(): Long = now
    }

    private class FakeDistanceSource(var meters: Double?) : DistanceSource {
        override fun start() = Unit
        override fun stop() = Unit
        override fun distanceMeters(): Double? = meters
    }

    private class RecordingNetwork {
        var calls = 0
            private set

        fun fetchPlan() {
            calls += 1
        }
    }
}

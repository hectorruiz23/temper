package dev.temper.intervals

import android.os.SystemClock
import dev.temper.clock.MonotonicClock
import dev.temper.clock.WallClock

object AndroidMonotonicClock : MonotonicClock {
    override fun elapsedRealtimeMs(): Long = SystemClock.elapsedRealtime()
}

object SystemWallClock : WallClock {
    override fun nowMs(): Long = System.currentTimeMillis()
}

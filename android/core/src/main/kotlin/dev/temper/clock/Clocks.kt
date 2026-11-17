package dev.temper.clock

fun interface MonotonicClock {
    fun elapsedRealtimeMs(): Long
}

fun interface WallClock {
    fun nowMs(): Long
}

interface DistanceSource {
    fun start()
    fun stop()
    fun distanceMeters(): Double?
}

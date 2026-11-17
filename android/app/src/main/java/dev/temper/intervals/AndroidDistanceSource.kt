package dev.temper.intervals

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Looper
import android.os.SystemClock
import androidx.core.content.ContextCompat
import dev.temper.clock.DistanceSource

class AndroidDistanceSource(private val context: Context) : DistanceSource {
    private val manager = context.getSystemService(LocationManager::class.java)
    private val listener = LocationListener { location -> onLocation(location) }
    private var started = false
    private var totalMeters = 0.0
    private var lastLocation: Location? = null
    private var lastFixAtMs = 0L

    override fun start() {
        if (started) return
        started = true
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            return
        }
        if (!manager.isProviderEnabled(LocationManager.GPS_PROVIDER)) return
        manager.requestLocationUpdates(LocationManager.GPS_PROVIDER, 1_000L, 3f, listener, Looper.getMainLooper())
    }

    override fun stop() {
        if (!started) return
        started = false
        runCatching { manager.removeUpdates(listener) }
        lastLocation = null
    }

    override fun distanceMeters(): Double? {
        if (!started) return null
        if (lastLocation == null) return null
        if (SystemClock.elapsedRealtime() - lastFixAtMs > 10_000L) return null
        return totalMeters
    }

    private fun onLocation(location: Location) {
        if (location.accuracy > 50f) return
        val previous = lastLocation
        if (previous != null) {
            val result = FloatArray(1)
            Location.distanceBetween(
                previous.latitude,
                previous.longitude,
                location.latitude,
                location.longitude,
                result,
            )
            if (result[0] >= 1f) totalMeters += result[0].toDouble()
        }
        lastLocation = location
        lastFixAtMs = SystemClock.elapsedRealtime()
    }
}

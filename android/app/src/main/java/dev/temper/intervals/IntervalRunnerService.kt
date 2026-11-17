package dev.temper.intervals

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.media.AudioManager
import android.media.ToneGenerator
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.os.VibrationEffect
import android.os.Vibrator
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import dev.temper.AppContainer
import dev.temper.MainActivity
import dev.temper.R
import dev.temper.TemperApplication
import dev.temper.api.WriteState
import dev.temper.clock.DistanceSource
import dev.temper.runner.IntervalRunner
import dev.temper.runner.RunnerSnapshot
import dev.temper.runner.RunnerStatus
import dev.temper.runner.RunnerTransition
import java.util.Locale

class IntervalRunnerService : android.app.Service() {
    private val handler = Handler(Looper.getMainLooper())
    private var runner: IntervalRunner? = null
    private var distanceSource: AndroidDistanceSource? = null
    private var wakeLock: PowerManager.WakeLock? = null
    private var toneGenerator: ToneGenerator? = null
    private var finishHandled = false
    private var foregroundStarted = false

    private val container: AppContainer get() = (application as TemperApplication).container
    private val controller: RunnerController get() = container.runnerController
    private val notificationManager: NotificationManager get() = getSystemService(NotificationManager::class.java)

    private val tickRunnable = object : Runnable {
        override fun run() {
            val runner = runner ?: return
            val tick = runner.tick()
            controller.publish(tick.snapshot)
            postCues(tick.transitions)
            updateNotification(tick.snapshot)
            when {
                tick.snapshot.finished -> finishRun(tick.snapshot)
                tick.snapshot.status == RunnerStatus.Running -> scheduleTick()
            }
        }
    }

    override fun onCreate() {
        super.onCreate()
        createChannels()
        toneGenerator = runCatching { ToneGenerator(AudioManager.STREAM_NOTIFICATION, 80) }.getOrNull()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_PAUSE -> pauseRun()
            ACTION_RESUME -> resumeRun()
            ACTION_DISCARD -> discardRun()
            else -> if (runner == null) startRun()
        }
        return START_NOT_STICKY
    }

    override fun onBind(intent: Intent?): android.os.IBinder? = null

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        distanceSource?.stop()
        releaseWakeLock()
        toneGenerator?.release()
        toneGenerator = null
        super.onDestroy()
    }

    private fun startRun() {
        val launch = controller.launch.value
        if (launch == null) {
            stopSelf()
            return
        }
        val needsDistance = launch.steps.any { it.isDistance }
        val source: DistanceSource = if (needsDistance && hasFineLocation()) {
            AndroidDistanceSource(this).also { distanceSource = it }
        } else {
            UnavailableDistanceSource
        }
        val runner = IntervalRunner(launch.steps, AndroidMonotonicClock, SystemWallClock, source)
        this.runner = runner
        acquireWakeLock()
        val initial = runner.snapshot()
        startForeground(initial, needsDistance)
        runner.start()
        controller.publish(runner.snapshot())
        updateNotification(runner.snapshot())
        scheduleTick()
    }

    private fun pauseRun() {
        val runner = runner ?: return
        val transitions = runner.pause()
        val snapshot = runner.snapshot()
        controller.publish(snapshot)
        postCues(transitions)
        updateNotification(snapshot)
        handler.removeCallbacks(tickRunnable)
        if (snapshot.finished) finishRun(snapshot)
    }

    private fun resumeRun() {
        val runner = runner ?: return
        val transitions = runner.resume()
        val snapshot = runner.snapshot()
        controller.publish(snapshot)
        postCues(transitions)
        updateNotification(snapshot)
        if (snapshot.status == RunnerStatus.Running) scheduleTick()
    }

    private fun discardRun() {
        handler.removeCallbacksAndMessages(null)
        distanceSource?.stop()
        releaseWakeLock()
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun scheduleTick() {
        handler.removeCallbacks(tickRunnable)
        handler.postDelayed(tickRunnable, 250L)
    }

    private fun finishRun(snapshot: RunnerSnapshot) {
        if (finishHandled) return
        finishHandled = true
        handler.removeCallbacks(tickRunnable)
        releaseWakeLock()
        val launch = controller.launch.value
        if (launch != null) {
            val completion = RunnerCompletion(
                sessionId = launch.sessionId,
                expectedRevision = launch.sessionRevision,
                completedAtMs = snapshot.finishedAtMs ?: SystemWallClock.nowMs(),
                totalElapsedMs = snapshot.runElapsedMs,
                stepsCompleted = snapshot.stepCount,
                stepCount = snapshot.stepCount,
            )
            controller.complete(completion)
            container.runnerReporter.report(container.configuration.value, completion)
        }
        updateNotification(snapshot)
        waitForReport(0)
    }

    private fun waitForReport(attempt: Int) {
        if (container.runnerReporter.state.value is WriteState.Sending && attempt < 100) {
            handler.postDelayed({ waitForReport(attempt + 1) }, 200L)
        } else {
            stopRunner()
        }
    }

    private fun stopRunner() {
        handler.removeCallbacksAndMessages(null)
        distanceSource?.stop()
        releaseWakeLock()
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun startForeground(snapshot: RunnerSnapshot, distanceRequired: Boolean) {
        ServiceCompat.startForeground(this, NOTIFICATION_ID, buildOngoing(snapshot), typeMask(distanceRequired))
        foregroundStarted = true
    }

    private fun typeMask(distanceRequired: Boolean): Int {
        val location = distanceRequired && hasFineLocation()
        if (Build.VERSION.SDK_INT >= 34) {
            var mask = ServiceInfo.FOREGROUND_SERVICE_TYPE_HEALTH
            if (location) mask = mask or ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION
            return mask
        }
        if (Build.VERSION.SDK_INT >= 29 && location) return ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION
        return 0
    }

    private fun hasFineLocation(): Boolean =
        ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED

    private fun acquireWakeLock() {
        if (wakeLock != null) return
        val power = getSystemService(PowerManager::class.java)
        wakeLock = power.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "temper:interval-runner").apply {
            setReferenceCounted(false)
            acquire(2 * 60 * 60 * 1_000L)
        }
    }

    private fun releaseWakeLock() {
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
    }

    private fun updateNotification(snapshot: RunnerSnapshot) {
        if (!foregroundStarted) return
        notificationManager.notify(NOTIFICATION_ID, buildOngoing(snapshot))
    }

    private fun buildOngoing(snapshot: RunnerSnapshot): Notification {
        val step = snapshot.currentStep
        val detail = when {
            snapshot.finished -> "Finished · ${formatDuration(snapshot.runElapsedMs)}"
            step.durationMs != null -> "${formatDuration(snapshot.stepRemainingMs ?: 0L)} left"
            !snapshot.distanceAvailable -> "Waiting for location"
            else -> "${formatMeters(snapshot.stepDistanceRemainingM ?: 0.0)} to go"
        }
        val builder = NotificationCompat.Builder(this, RUNNER_CHANNEL)
            .setSmallIcon(R.drawable.ic_runner)
            .setContentTitle(if (snapshot.finished) "Session finished" else step.label)
            .setContentText("Step ${snapshot.stepNumber} of ${snapshot.stepCount} · $detail")
            .setOngoing(!snapshot.finished)
            .setOnlyAlertOnce(true)
            .setCategory(NotificationCompat.CATEGORY_STOPWATCH)
            .setContentIntent(openAppPendingIntent())
        if (!snapshot.finished) {
            val paused = snapshot.status == RunnerStatus.Paused
            builder.addAction(
                0,
                if (paused) "Resume" else "Pause",
                actionPendingIntent(if (paused) ACTION_RESUME else ACTION_PAUSE),
            )
        }
        return builder.build()
    }

    private fun postCues(transitions: List<RunnerTransition>) {
        transitions.forEach { transition ->
            vibrate()
            toneGenerator?.startTone(ToneGenerator.TONE_PROP_BEEP, 120)
            val title = if (transition.finished) "Session finished" else "Next · ${transition.label}"
            val notification = NotificationCompat.Builder(this, CUES_CHANNEL)
                .setSmallIcon(R.drawable.ic_runner)
                .setContentTitle(title)
                .setContentText("Step ${transition.toIndex + 1}")
                .setAutoCancel(true)
                .setCategory(NotificationCompat.CATEGORY_ALARM)
                .setContentIntent(openAppPendingIntent())
                .build()
            notificationManager.notify(CUE_NOTIFICATION_ID, notification)
        }
    }

    private fun vibrate() {
        val vibrator = getSystemService(Vibrator::class.java) ?: return
        vibrator.vibrate(VibrationEffect.createOneShot(120L, VibrationEffect.DEFAULT_AMPLITUDE))
    }

    private fun actionPendingIntent(action: String): PendingIntent {
        val intent = Intent(this, IntervalRunnerService::class.java).setAction(action)
        return PendingIntent.getService(
            this,
            action.hashCode(),
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    private fun openAppPendingIntent(): PendingIntent {
        val intent = Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP)
        return PendingIntent.getActivity(
            this,
            0,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    private fun createChannels() {
        val runner = NotificationChannel(RUNNER_CHANNEL, "Interval run", NotificationManager.IMPORTANCE_LOW).apply {
            setShowBadge(false)
        }
        val cues = NotificationChannel(CUES_CHANNEL, "Interval cues", NotificationManager.IMPORTANCE_HIGH).apply {
            enableVibration(false)
        }
        notificationManager.createNotificationChannels(listOf(runner, cues))
    }

    private fun formatDuration(milliseconds: Long): String {
        val totalSeconds = milliseconds / 1_000L
        val hours = totalSeconds / 3_600L
        val minutes = (totalSeconds % 3_600L) / 60L
        val seconds = totalSeconds % 60L
        return if (hours > 0L) {
            String.format(Locale.US, "%d:%02d:%02d", hours, minutes, seconds)
        } else {
            String.format(Locale.US, "%d:%02d", minutes, seconds)
        }
    }

    private fun formatMeters(value: Double): String =
        if (value % 1.0 == 0.0) "${value.toInt()} m" else String.format(Locale.US, "%.1f m", value)

    private object UnavailableDistanceSource : DistanceSource {
        override fun start() = Unit
        override fun stop() = Unit
        override fun distanceMeters(): Double? = null
    }

    companion object {
        const val ACTION_PAUSE = "dev.temper.runner.PAUSE"
        const val ACTION_RESUME = "dev.temper.runner.RESUME"
        const val ACTION_DISCARD = "dev.temper.runner.DISCARD"

        private const val RUNNER_CHANNEL = "runner"
        private const val CUES_CHANNEL = "runner_cues"
        private const val NOTIFICATION_ID = 1_001
        private const val CUE_NOTIFICATION_ID = 1_002
    }
}

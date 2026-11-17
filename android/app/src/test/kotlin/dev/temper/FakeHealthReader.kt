package dev.temper

import dev.temper.health.HealthAvailability
import dev.temper.health.HealthPermission
import dev.temper.health.HealthReader
import dev.temper.health.HealthRecords
import java.time.LocalDate
import java.time.ZoneId

class FakeHealthReader(
    var status: HealthAvailability = HealthAvailability.Available,
    var granted: Set<HealthPermission> = HealthPermission.all.toSet(),
    var records: HealthRecords = HealthRecords(),
) : HealthReader {
    var reads: Int = 0
        private set

    override suspend fun availability(): HealthAvailability = status

    override suspend fun grantedPermissions(): Set<HealthPermission> = granted

    override suspend fun read(
        day: LocalDate,
        zone: ZoneId,
        permissions: Set<HealthPermission>,
    ): HealthRecords {
        reads += 1
        return records
    }
}

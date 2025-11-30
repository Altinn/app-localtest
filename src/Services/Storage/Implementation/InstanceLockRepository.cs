using System.Text.Json;
using Altinn.Platform.Storage.Models;
using Altinn.Platform.Storage.Repository;
using LocalTest.Configuration;
using Microsoft.Extensions.Options;

namespace LocalTest.Services.Storage.Implementation;

public class InstanceLockRepository(
    IOptions<LocalPlatformSettings> localPlatformSettings,
    TimeProvider timeProvider
) : IInstanceLockRepository
{
    private readonly PartitionedAsyncLock _lock = new();

    private Task<IDisposable> Lock(Guid instanceGuid) => _lock.Lock(instanceGuid);

    private readonly LocalPlatformSettings _localPlatformSettings = localPlatformSettings.Value;

    public async Task<(AcquireLockResult Result, Guid? LockId)> TryAcquireLock(
        Guid instanceGuid,
        int ttlSeconds,
        string userId,
        CancellationToken cancellationToken
    )
    {
        using var _ = await Lock(instanceGuid);

        Directory.CreateDirectory(GetProcessLockFolder());

        foreach (var lockFile in Directory.EnumerateFiles(GetProcessLockFolder(), $"{instanceGuid}_*.json"))
        {
            await using FileStream openStream = File.OpenRead(lockFile);
            var existingLockData = await JsonSerializer.DeserializeAsync<InstanceLock>(
                openStream,
                cancellationToken: cancellationToken);

            if (existingLockData.LockedUntil > timeProvider.GetUtcNow())
            {
                return (AcquireLockResult.LockAlreadyHeld, null);
            }
        }

        var processLockId = Guid.NewGuid();

        var now = timeProvider.GetUtcNow();
        var lockData = new InstanceLock
        {
            Id = processLockId,
            InstanceGuid = instanceGuid,
            LockedAt = now,
            LockedUntil = now.AddSeconds(ttlSeconds),
            LockedBy = userId
        };

        string path = GetProcessLockPath(instanceGuid, processLockId);

        await using FileStream createStream = File.Create(path);
        await JsonSerializer.SerializeAsync(
            createStream,
            lockData,
            cancellationToken: cancellationToken
        );

        return (AcquireLockResult.Success, processLockId);
    }

    public async Task<UpdateLockResult> TryUpdateLockExpiration(
        Guid lockId,
        Guid instanceGuid,
        int ttlSeconds,
        CancellationToken cancellationToken = default
    )
    {
        using var _ = await Lock(instanceGuid);

        var lockFile = GetProcessLockPath(instanceGuid, lockId);
        if (!File.Exists(lockFile))
        {
            return UpdateLockResult.LockNotFound;
        }

        await using var fileStream = File.Open(lockFile, FileMode.Open, FileAccess.ReadWrite);

        var existingLockData = await JsonSerializer.DeserializeAsync<InstanceLock>(
            fileStream,
            cancellationToken: cancellationToken);

        var now = timeProvider.GetUtcNow();

        if (existingLockData.LockedUntil <= now)
        {
            return UpdateLockResult.LockExpired;
        }

        var lockData = new InstanceLock
        {
            Id = existingLockData.Id,
            InstanceGuid = existingLockData.InstanceGuid,
            LockedAt = existingLockData.LockedAt,
            LockedUntil = now.AddSeconds(ttlSeconds),
            LockedBy = existingLockData.LockedBy
        };

        await JsonSerializer.SerializeAsync(
            fileStream,
            lockData,
            cancellationToken: cancellationToken
        );

        return UpdateLockResult.Success;
    }

    public Task<InstanceLock> Get(Guid lockId, CancellationToken cancellationToken = default)
    {
        throw new NotImplementedException();
    }

    private string GetProcessLockPath(Guid instanceGuid, Guid processLockId)
    {
        return $"{GetProcessLockFolder()}{instanceGuid}_{processLockId}.json";
    }

    private string GetProcessLockFolder()
    {
        return _localPlatformSettings.LocalTestingStorageBasePath
            + _localPlatformSettings.DocumentDbFolder
            + _localPlatformSettings.ProcessLockFolder;
    }
}

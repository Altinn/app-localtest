#nullable enable

using Altinn.Platform.Storage.Models;

namespace Altinn.Platform.Storage.Repository;

/// <summary>
/// The repository to handle instance locks
/// </summary>
public interface IInstanceLockRepository
{
    /// <summary>
    /// Attempts to acquire a lock for an instance.
    /// </summary>
    /// <param name="instanceInternalId">The instance internal ID</param>
    /// <param name="ttlSeconds">Lock time to live in seconds</param>
    /// <param name="userId">The ID of the user acquiring the lock</param>
    /// <param name="cancellationToken">CancellationToken</param>
    /// <returns>A tuple containing the result of the operation and the lock ID if successful.</returns>
    Task<(AcquireLockResult Result, Guid? LockId)> TryAcquireLock(
        Guid instanceGuid,
        int ttlSeconds,
        string userId,
        CancellationToken cancellationToken = default
    );

    /// <summary>
    /// Tries to update the expiration of an existing lock. Fails if the lock doesn't exist or is no longer active.
    /// </summary>
    /// <param name="lockId">The lock ID</param>
    /// <param name="instanceInternalId">The instance internal ID</param>
    /// <param name="ttlSeconds">New time to live in seconds</param>
    /// <param name="cancellationToken">CancellationToken</param>
    /// <returns>The result of the operation.</returns>
    Task<UpdateLockResult> TryUpdateLockExpiration(
        Guid lockId,
        Guid instanceGuid,
        int ttlSeconds,
        CancellationToken cancellationToken = default
    );

    /// <summary>
    /// Gets the details of a lock
    /// </summary>
    /// <param name="lockId">The lock ID</param>
    /// <param name="cancellationToken">CancellationToken</param>
    /// <returns>The lock details if they exist, null otherwise</returns>
    Task<InstanceLock?> Get(Guid lockId, CancellationToken cancellationToken = default);
}

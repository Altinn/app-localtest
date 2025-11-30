#nullable enable
using System.Diagnostics;
using System.Net.Http.Headers;
using Altinn.Platform.Storage.Authorization;
using Altinn.Platform.Storage.Helpers;
using Altinn.Platform.Storage.Interface.Models;
using Altinn.Platform.Storage.Repository;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;

namespace Altinn.Platform.Storage.Controllers;

/// <summary>
/// Handles operations for the application instance locks
/// </summary>
[Route("storage/api/v1/instances/{instanceOwnerPartyId:int}/{instanceGuid:guid}/lock")]
[ApiController]
public class InstanceLockController : ControllerBase
{
    private readonly IInstanceRepository _instanceRepository;
    private readonly IInstanceLockRepository _instanceLockRepository;
    private readonly IAuthorization _authorizationService;

    /// <summary>
    /// Initializes a new instance of the <see cref="InstanceLockController"/> class
    /// </summary>
    /// <param name="instanceRepository">the instance repository handler</param>
    /// <param name="instanceLockRepository">the instance lock repository</param>
    /// <param name="authorizationService">the authorization service</param>
    public InstanceLockController(
        IInstanceRepository instanceRepository,
        IInstanceLockRepository instanceLockRepository,
        IAuthorization authorizationService
    )
    {
        _instanceRepository = instanceRepository;
        _instanceLockRepository = instanceLockRepository;
        _authorizationService = authorizationService;
    }

    /// <summary>
    /// Attempts to acquire a lock for an instance.
    /// </summary>
    /// <param name="instanceOwnerPartyId">The party id of the instance owner.</param>
    /// <param name="instanceGuid">The id of the instance to lock.</param>
    /// <param name="request">The lock request containing expiration time.</param>
    /// <param name="cancellationToken">CancellationToken</param>
    /// <returns>The lock response with lock key if successful, or Conflict if lock is already held.</returns>
    [Authorize]
    [HttpPost]
    [Consumes("application/json")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    [ProducesResponseType(StatusCodes.Status400BadRequest)]
    [ProducesResponseType(StatusCodes.Status401Unauthorized)]
    [ProducesResponseType(StatusCodes.Status403Forbidden)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    [ProducesResponseType(StatusCodes.Status409Conflict)]
    [Produces("application/json")]
    public async Task<ActionResult<InstanceLockResponse>> AcquireInstanceLock(
        int instanceOwnerPartyId,
        Guid instanceGuid,
        [FromBody] InstanceLockRequest request,
        CancellationToken cancellationToken
    )
    {
        if (request.TtlSeconds < 0)
        {
            return Problem(
                detail: "TtlSeconds cannot be negative.",
                statusCode: StatusCodes.Status400BadRequest
            );
        }

        var userOrOrgNo = User.GetUserOrOrgNo();

        if (userOrOrgNo is null)
        {
            return Problem(
                detail: "User identity could not be determined.",
                statusCode: StatusCodes.Status401Unauthorized
            );
        }

        var instance = await _instanceRepository.GetOne(
            instanceOwnerPartyId,
            instanceGuid
        );

        if (instance is null || instance.InstanceOwner.PartyId != instanceOwnerPartyId.ToString())
        {
            return Problem(
                detail: "Instance not found.",
                statusCode: StatusCodes.Status404NotFound
            );
        }

        var atLeastOneActionAuthorized = await AuthorizeInstanceLock(instance);

        if (!atLeastOneActionAuthorized)
        {
            return Problem(
                detail: "Not authorized to acquire instance lock.",
                statusCode: StatusCodes.Status403Forbidden
            );
        }

        var (result, lockId) = await _instanceLockRepository.TryAcquireLock(
            instanceGuid,
            request.TtlSeconds,
            userOrOrgNo,
            cancellationToken
        );

        return result switch
        {
            AcquireLockResult.Success => Ok(
                new InstanceLockResponse
                {
                    LockToken = Convert.ToBase64String(lockId!.Value.ToByteArray()),
                }
            ),
            AcquireLockResult.LockAlreadyHeld => Problem(
                detail: "Lock is already held for this instance.",
                statusCode: StatusCodes.Status409Conflict
            ),
            _ => throw new UnreachableException(),
        };
    }

    /// <summary>
    /// Updates TTL on an instance lock.
    /// </summary>
    /// <param name="instanceOwnerPartyId">The party id of the instance owner.</param>
    /// <param name="instanceGuid">The id of the instance to lock.</param>
    /// <param name="request">The lock request (TTL should be 0 for release).</param>
    /// <param name="cancellationToken">CancellationToken</param>
    /// <returns>NoContent if successful.</returns>
    [HttpPatch]
    [Consumes("application/json")]
    [ProducesResponseType(StatusCodes.Status204NoContent)]
    [ProducesResponseType(StatusCodes.Status400BadRequest)]
    [ProducesResponseType(StatusCodes.Status401Unauthorized)]
    [ProducesResponseType(StatusCodes.Status404NotFound)]
    [ProducesResponseType(StatusCodes.Status422UnprocessableEntity)]
    [Produces("application/json")]
    public async Task<ActionResult> UpdateInstanceLock(
        int instanceOwnerPartyId,
        Guid instanceGuid,
        [FromBody] InstanceLockRequest request,
        CancellationToken cancellationToken
    )
    {
        if (
            !AuthenticationHeaderValue.TryParse(
                HttpContext.Request.Headers.Authorization,
                out var parsedHeader
            )
            || parsedHeader.Scheme != "Bearer"
            || string.IsNullOrEmpty(parsedHeader.Parameter)
        )
        {
            return Problem(
                detail: "Authorization header value missing or in wrong format.",
                statusCode: StatusCodes.Status401Unauthorized
            );
        }
        var guidBytes = new byte[16];
        if (
            !Convert.TryFromBase64String(parsedHeader.Parameter, guidBytes, out var bytesWritten)
            || bytesWritten != 16
        )
        {
            return Problem(
                detail: "Could not parse token.",
                statusCode: StatusCodes.Status401Unauthorized
            );
        }

        var lockId = new Guid(guidBytes);

        if (request.TtlSeconds < 0)
        {
            return Problem(
                detail: "TtlSeconds cannot be negative.",
                statusCode: StatusCodes.Status400BadRequest
            );
        }

        var instance = await _instanceRepository.GetOne(
            instanceOwnerPartyId,
            instanceGuid
        );

        if (instance is null || instance.InstanceOwner.PartyId != instanceOwnerPartyId.ToString())
        {
            return Problem(
                detail: "Instance not found.",
                statusCode: StatusCodes.Status404NotFound
            );
        }

        var result = await _instanceLockRepository.TryUpdateLockExpiration(
            lockId,
            instanceGuid,
            request.TtlSeconds,
            cancellationToken
        );

        return result switch
        {
            UpdateLockResult.Success => NoContent(),
            UpdateLockResult.LockNotFound => Problem(
                detail: "Lock not found.",
                statusCode: StatusCodes.Status404NotFound
            ),
            UpdateLockResult.LockExpired => Problem(
                detail: "Lock has expired.",
                statusCode: StatusCodes.Status422UnprocessableEntity
            ),
            _ => throw new UnreachableException(),
        };
    }

    private async Task<bool> AuthorizeInstanceLock(Instance existingInstance)
    {
        string[] actionsThatAllowLock =
        [
            .. ProcessController.GetActionsThatAllowProcessNextForTaskType(
                existingInstance.Process?.CurrentTask?.AltinnTaskType
            ),
            "reject",
        ];
        var taskId = existingInstance.Process?.CurrentTask?.ElementId;

        foreach (string action in actionsThatAllowLock)
        {
            bool actionIsAuthorized = await _authorizationService.AuthorizeInstanceAction(
                existingInstance,
                action,
                taskId
            );
            if (actionIsAuthorized)
            {
                return true;
            }
        }

        return false;
    }
}

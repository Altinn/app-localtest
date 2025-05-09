using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using System.Web;
using Altinn.AccessManagement.Core.Models;
using Altinn.Authorization.ABAC.Xacml.JsonProfile;
using Altinn.Common.PEP.Helpers;
using Altinn.Common.PEP.Interfaces;

using Altinn.Platform.Storage.Clients;
using Altinn.Platform.Storage.Helpers;
using Altinn.Platform.Storage.Interface.Enums;
using Altinn.Platform.Storage.Interface.Models;
using Altinn.Platform.Storage.Repository;

using LocalTest.Configuration;

using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.Extensions;
using Microsoft.AspNetCore.Mvc;
using Microsoft.AspNetCore.WebUtilities;

using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;
using Microsoft.Extensions.Primitives;

using Newtonsoft.Json;

using Substatus = Altinn.Platform.Storage.Interface.Models.Substatus;

namespace Altinn.Platform.Storage.Controllers
{
    /// <summary>
    /// Handles operations for the application instance resource
    /// </summary>
    [Route("storage/api/v1/instances")]
    [ApiController]
    public class InstancesController : ControllerBase
    {
        private readonly List<string> _instanceReadScope = new List<string>()
        {
            "altinn:serviceowner/instances.read"
        };

        private readonly IInstanceRepository _instanceRepository;
        private readonly IInstanceEventRepository _instanceEventRepository;
        private readonly IApplicationRepository _applicationRepository;
        private readonly IPartiesWithInstancesClient _partiesWithInstancesClient;
        private readonly ILogger _logger;
        private readonly IPDP _pdp;
        private readonly AuthorizationHelper _authzHelper;
        private readonly string _storageBaseAndHost;
        private readonly GeneralSettings _generalSettings;

        /// <summary>
        /// Initializes a new instance of the <see cref="InstancesController"/> class
        /// </summary>
        /// <param name="instanceRepository">the instance repository handler</param>
        /// <param name="instanceEventRepository">the instance event repository service</param>
        /// <param name="applicationRepository">the application repository handler</param>
        /// <param name="partiesWithInstancesClient">An implementation of <see cref="IPartiesWithInstancesClient"/> that can be used to send information to SBL.</param>
        /// <param name="logger">the logger</param>
        /// <param name="authzLogger">the logger for the authorization helper</param>
        /// <param name="pdp">the policy decision point.</param>
        /// <param name="settings">the general settings.</param>
        public InstancesController(
            IInstanceRepository instanceRepository,
            IInstanceEventRepository instanceEventRepository,
            IApplicationRepository applicationRepository,
            IPartiesWithInstancesClient partiesWithInstancesClient,
            ILogger<InstancesController> logger,
            ILogger<AuthorizationHelper> authzLogger,
            IPDP pdp,
            IOptions<GeneralSettings> settings)
        {
            _instanceRepository = instanceRepository;
            _instanceEventRepository = instanceEventRepository;
            _applicationRepository = applicationRepository;
            _partiesWithInstancesClient = partiesWithInstancesClient;
            _pdp = pdp;
            _logger = logger;
            _storageBaseAndHost = $"{settings.Value.Hostname}/storage/api/v1/";
            _authzHelper = new AuthorizationHelper(pdp, authzLogger);
            _generalSettings = settings.Value;
        }

        /// <summary>
        /// Get all instances that match the given query parameters. Parameters can be combined. Unknown or illegal parameter values will result in 400 - bad request.
        /// </summary>
        /// <param name="org">application owner.</param>
        /// <param name="appId">application id.</param>
        /// <param name="currentTaskId">Running process current task id.</param>
        /// <param name="processIsComplete">Is process complete.</param>
        /// <param name="processEndEvent">Process end state.</param>
        /// <param name="processEnded">Process ended value.</param>
        /// <param name="instanceOwnerPartyId">Instance owner id.</param>
        /// <param name="lastChanged">Last changed date.</param>
        /// <param name="created">Created time.</param>
        /// <param name="visibleAfter">The visible after date time.</param>
        /// <param name="dueBefore">The due before date time.</param>
        /// <param name="excludeConfirmedBy">A string that will hide instances already confirmed by stakeholder.</param>
        /// <param name="isSoftDeleted">Is the instance soft deleted.</param>
        /// <param name="isHardDeleted">Is the instance hard deleted.</param>
        /// <param name="isArchived">Is the instance archived.</param>
        /// <param name="continuationToken">Continuation token.</param>
        /// <param name="size">The page size.</param>
        /// <returns>List of all instances for given instance owner.</returns>
        /// <!-- GET /instances?org=tdd or GET /instances?appId=tdd/app2 -->
        [Authorize]
        [HttpGet]
        [ProducesResponseType(StatusCodes.Status200OK)]
        [ProducesResponseType(StatusCodes.Status400BadRequest)]
        [Produces("application/json")]
        public async Task<ActionResult<QueryResponse<Instance>>> GetInstances(
            [FromQuery] string org,
            [FromQuery] string appId,
            [FromQuery(Name = "process.currentTask")] string currentTaskId,
            [FromQuery(Name = "process.isComplete")] bool? processIsComplete,
            [FromQuery(Name = "process.endEvent")] string processEndEvent,
            [FromQuery(Name = "process.ended")] string processEnded,
            [FromQuery(Name = "instanceOwner.partyId")] int? instanceOwnerPartyId,
            [FromQuery] string lastChanged,
            [FromQuery] string created,
            [FromQuery(Name = "visibleAfter")] string visibleAfter,
            [FromQuery] string dueBefore,
            [FromQuery] string excludeConfirmedBy,
            [FromQuery(Name = "status.isSoftDeleted")] bool isSoftDeleted,
            [FromQuery(Name = "status.isHardDeleted")] bool isHardDeleted,
            [FromQuery(Name = "status.isArchived")] bool isArchived,
            string continuationToken,
            int? size)
        {
            int pageSize = size ?? 100;
            string selfContinuationToken = null;

            // if user is org
            string orgClaim = User.GetOrg();            
            int? userId = User.GetUserId();
            SystemUserClaim systemUser = User.GetSystemUser();

            if (orgClaim != null)
            {
                if (!_authzHelper.ContainsRequiredScope(_instanceReadScope, User))
                {
                    return Forbid();
                }

                if (string.IsNullOrEmpty(org) && string.IsNullOrEmpty(appId))
                {
                    return BadRequest("Org or AppId must be defined.");
                }

                org = string.IsNullOrEmpty(org) ? appId.Split('/')[0] : org;

                if (!orgClaim.Equals(org, StringComparison.InvariantCultureIgnoreCase))
                {
                    return Forbid();
                }
            }
            else if (userId is not null || systemUser is not null)
            {
                if (instanceOwnerPartyId == null)
                {
                    return BadRequest("InstanceOwnerPartyId must be defined.");
                }
            }
            else
            {
                return BadRequest();
            }

            if (!string.IsNullOrEmpty(continuationToken))
            {
                selfContinuationToken = continuationToken;
                continuationToken = HttpUtility.UrlDecode(continuationToken);
            }

            Dictionary<string, StringValues> queryParams = QueryHelpers.ParseQuery(Request.QueryString.Value);

            bool appOwnerRequestingInstances = User.HasServiceOwnerScope();
            // filter out hard deleted instances if it isn't appOwner requesting instances
            if (!appOwnerRequestingInstances)
            {
                bool requestingHardDeleted = false;

                if (queryParams.TryGetValue("status.isHardDeleted", out StringValues hardDeletedQueryValue))
                {
                    _ = bool.TryParse(hardDeletedQueryValue.Single(), out requestingHardDeleted);
                }

                if (requestingHardDeleted)
                {
                    return new QueryResponse<Instance>()
                    {
                        Instances = new(),
                        Self = BuildRequestLink(selfContinuationToken)
                    };
                }
                else if (!queryParams.ContainsKey("status.isHardDeleted"))
                {
                    queryParams["status.isHardDeleted"] = "false";
                }
            }

            try
            {
                InstanceQueryResponse result = await _instanceRepository.GetInstancesFromQuery(queryParams, continuationToken, pageSize);

                if (!string.IsNullOrEmpty(result.Exception))
                {
                    return BadRequest(result.Exception);
                }

                if (!appOwnerRequestingInstances)
                {
                    foreach (Instance instance in result.Instances)
                    {
                        FilterOutDeletedDataElements(instance);
                    }

                    result.Instances = await _authzHelper.AuthorizeInstances(User, result.Instances);
                    result.Count = result.Instances.Count;
                }

                string nextContinuationToken = HttpUtility.UrlEncode(result.ContinuationToken);
                result.ContinuationToken = null;

                QueryResponse<Instance> response = new()
                {
                    Instances = result.Instances,
                    Count = result.Instances.Count,
                    Self = BuildRequestLink(selfContinuationToken, queryParams)
                };

                if (!string.IsNullOrEmpty(nextContinuationToken))
                {
                    response.Next = BuildRequestLink(nextContinuationToken, queryParams);
                }

                // add self links to platform
                result.Instances.ForEach(i => i.SetPlatformSelfLinks(_storageBaseAndHost));

                return Ok(response);
            }
            catch (Exception e)
            {
                _logger.LogError(e, "Unable to perform query on instances");
                return StatusCode(500, $"Unable to perform query on instances due to: {e.Message}");
            }
        }

        /// <summary>
        /// Gets a specific instance with the given instance id.
        /// </summary>
        /// <param name="instanceOwnerPartyId">The party id of the instance owner.</param>
        /// <param name="instanceGuid">The id of the instance to retrieve.</param>
        /// <returns>The information about the specific instance.</returns>
        [Authorize(Policy = AuthzConstants.POLICY_INSTANCE_READ)]
        [HttpGet("{instanceOwnerPartyId:int}/{instanceGuid:guid}")]
        [ProducesResponseType(StatusCodes.Status200OK)]
        [ProducesResponseType(StatusCodes.Status404NotFound)]
        [Produces("application/json")]
        public async Task<ActionResult<Instance>> Get(int instanceOwnerPartyId, Guid instanceGuid)
        {
            try
            {
                Instance result = await _instanceRepository.GetOne(instanceOwnerPartyId, instanceGuid);

                bool appOwnerRequestingElement = User.GetOrg() == result.Org;
                if (!appOwnerRequestingElement)
                {
                    FilterOutDeletedDataElements(result);
                }

                result.SetPlatformSelfLinks(_storageBaseAndHost);

                return Ok(result);
            }
            catch (Exception e)
            {
                return NotFound($"Unable to find instance {instanceOwnerPartyId}/{instanceGuid}: {e}");
            }
        }

        /// <summary>
        /// Inserts new instance into the instance collection.
        /// </summary>
        /// <param name="appId">the application id</param>
        /// <param name="instance">The instance details to store.</param>
        /// <returns>The stored instance.</returns>
        /// <!-- POST /instances?appId={appId} -->
        [Authorize]
        [HttpPost]
        [Consumes("application/json")]
        [ProducesResponseType(StatusCodes.Status201Created)]
        [ProducesResponseType(StatusCodes.Status400BadRequest)]
        [Produces("application/json")]
        public async Task<ActionResult<Instance>> Post(string appId, [FromBody] Instance instance)
        {
            Application appInfo = null;
            ActionResult appInfoError;
            int instanceOwnerPartyId = int.Parse(instance.InstanceOwner.PartyId);

            try
            {
                (appInfo, appInfoError) = await GetApplicationOrErrorAsync(appId);
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Something went wrong during GetApplicationOrErrorAsync for application id: {appId} AppInfo: {appInfo}", appId, appInfo?.ToString());
                throw;
            }

            if (appInfoError != null)
            {
                return appInfoError;
            }

            if (string.IsNullOrWhiteSpace(instance.InstanceOwner.PartyId))
            {
                return BadRequest("Cannot create an instance without an instanceOwner.PartyId.");
            }

            // Checking that user is authorized to instantiate.
            XacmlJsonRequestRoot request;
            try
            {
                request = DecisionHelper.CreateDecisionRequest(appInfo.Org, appInfo.Id.Split('/')[1], HttpContext.User, "instantiate", instanceOwnerPartyId, null);
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Something went wrong during CreateDecisionRequest for application id: {appId} AppInfo: {appInfo}", appId, appInfo?.ToString());
                throw;
            }

            XacmlJsonResponse response;
            try
            {
                response = await _pdp.GetDecisionForRequest(request);
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Something went wrong during GetDecisionForRequest for application id: {appId} AppInfo: {appInfo}", appId, appInfo?.ToString());
                throw;
            }

            if (response?.Response == null)
            {
                _logger.LogInformation("// Instances Controller // Authorization of instantiation failed with request: {request}.", JsonConvert.SerializeObject(request));
                return Forbid();
            }

            bool authorized;
            try
            {
                authorized = DecisionHelper.ValidatePdpDecision(response.Response, HttpContext.User);
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Something went wrong during ValidatePdpDecision for application id: {appId} AppInfo: {appInfo}", appId, appInfo?.ToString());
                throw;
            }

            if (!authorized)
            {
                return Forbid();
            }

            Instance storedInstance = new Instance();
            try
            {
                DateTime creationTime = DateTime.UtcNow;

                Instance instanceToCreate = CreateInstanceFromTemplate(appInfo, instance, creationTime, User.GetUserOrOrgNo());

                storedInstance = await _instanceRepository.Create(instanceToCreate);
                await DispatchEvent(InstanceEventType.Created, storedInstance);
                _logger.LogInformation("Created instance: {storedInstance.Id}", storedInstance.Id);
                storedInstance.SetPlatformSelfLinks(_storageBaseAndHost);

                await _partiesWithInstancesClient.SetHasAltinn3Instances(instanceOwnerPartyId);
                return Created(storedInstance.SelfLinks.Platform, storedInstance);
            }
            catch (Exception storageException)
            {
                _logger.LogError(storageException, "Unable to create {appId} instance for {instance.InstanceOwner.PartyId}", appId, instance.InstanceOwner.PartyId);

                // compensating action - delete instance
                if (storedInstance != null)
                {
                    await _instanceRepository.Delete(storedInstance);
                }

                _logger.LogError("Deleted instance {storedInstance.Id}", storedInstance?.Id);
                return StatusCode(500, $"Unable to create {appId} instance for {instance.InstanceOwner?.PartyId} due to {storageException.Message}");
            }
        }

        /// <summary>
        /// Delete an instance.
        /// </summary>
        /// <param name="instanceOwnerPartyId">The party id of the instance owner.</param>
        /// <param name="instanceGuid">The id of the instance that should be deleted.</param>
        /// <param name="hard">if true hard delete will take place. if false, the instance gets its status.softDelete attribute set to current date and time.</param>
        /// <returns>Information from the deleted instance.</returns>
        [Authorize(Policy = AuthzConstants.POLICY_INSTANCE_DELETE)]
        [HttpDelete("{instanceOwnerPartyId:int}/{instanceGuid:guid}")]
        [ProducesResponseType(StatusCodes.Status200OK)]
        [ProducesResponseType(StatusCodes.Status204NoContent)]
        [ProducesResponseType(StatusCodes.Status400BadRequest)]
        [ProducesResponseType(StatusCodes.Status404NotFound)]
        [Produces("application/json")]
        public async Task<ActionResult<Instance>> Delete(int instanceOwnerPartyId, Guid instanceGuid, [FromQuery] bool hard)
        {
            Instance instance;

            instance = await _instanceRepository.GetOne(instanceOwnerPartyId, instanceGuid);

            if (instance == null)
            {
                return NotFound($"Didn't find the object that should be deleted with instanceId={instanceOwnerPartyId}/{instanceGuid}");
            }

            DateTime now = DateTime.UtcNow;

            if (instance.Status == null)
            {
                instance.Status = new InstanceStatus();
            }

            if (hard)
            {
                instance.Status.IsHardDeleted = true;
                instance.Status.IsSoftDeleted = true;
                instance.Status.HardDeleted = now;
                instance.Status.SoftDeleted ??= now;
            }
            else
            {
                instance.Status.IsSoftDeleted = true;
                instance.Status.SoftDeleted = now;
            }

            instance.LastChangedBy = User.GetUserOrOrgNo();
            instance.LastChanged = now;

            try
            {
                Instance deletedInstance = await _instanceRepository.Update(instance);

                return Ok(deletedInstance);
            }
            catch (Exception e)
            {
                _logger.LogError(e, "Unexpected exception when deleting instance {instance.Id}", instance.Id);
                return StatusCode(500, $"Unexpected exception when deleting instance {instance.Id}: {e.Message}");
            }
        }

        /// <summary>
        /// Add complete confirmation.
        /// </summary>
        /// <remarks>
        /// Add to an instance that a given stakeholder considers the instance as no longer needed by them. The stakeholder has
        /// collected all the data and information they needed from the instance and expect no additional data to be added to it.
        /// The body of the request isn't used for anything despite this being a POST operation.
        /// </remarks>
        /// <param name="instanceOwnerPartyId">The party id of the instance owner.</param>
        /// <param name="instanceGuid">The id of the instance to confirm as complete.</param>
        /// <returns>Returns a list of the process events.</returns>
        [Authorize(Policy = AuthzConstants.POLICY_INSTANCE_COMPLETE)]
        [HttpPost("{instanceOwnerPartyId:int}/{instanceGuid:guid}/complete")]
        [ProducesResponseType(StatusCodes.Status200OK)]
        [Produces("application/json")]
        public async Task<ActionResult<Instance>> AddCompleteConfirmation(
            [FromRoute] int instanceOwnerPartyId,
            [FromRoute] Guid instanceGuid)
        {
            Instance instance = await _instanceRepository.GetOne(instanceOwnerPartyId, instanceGuid);

            string org = User.GetOrg();

            instance.CompleteConfirmations ??= new List<CompleteConfirmation>();
            if (instance.CompleteConfirmations.Any(cc => cc.StakeholderId == org))
            {
                instance.SetPlatformSelfLinks(_storageBaseAndHost);
                return Ok(instance);
            }

            instance.CompleteConfirmations.Add(new CompleteConfirmation { StakeholderId = org, ConfirmedOn = DateTime.UtcNow });
            instance.LastChanged = DateTime.UtcNow;
            instance.LastChangedBy = User.GetUserOrOrgNo();

            Instance updatedInstance;
            try
            {
                updatedInstance = await _instanceRepository.Update(instance);
                updatedInstance.SetPlatformSelfLinks(_storageBaseAndHost);
            }
            catch (Exception e)
            {
                _logger.LogError(e, "Unable to update instance {instanceOwnerPartyId}/{instanceGuid}", instanceOwnerPartyId, instanceGuid);
                return StatusCode(StatusCodes.Status500InternalServerError);
            }

            await DispatchEvent(InstanceEventType.ConfirmedComplete, updatedInstance);

            return Ok(updatedInstance);
        }

        /// <summary>
        /// Update instance read status.
        /// </summary>
        /// <param name="instanceOwnerPartyId">The party id of the instance owner.</param>
        /// <param name="instanceGuid">The id of the instance to confirm as complete.</param>
        /// <param name="status">The updated read status.</param>
        /// <returns>Returns the updated instance.</returns>
        [Authorize(Policy = AuthzConstants.POLICY_INSTANCE_READ)]
        [HttpPut("{instanceOwnerPartyId:int}/{instanceGuid:guid}/readstatus")]
        [ProducesResponseType(StatusCodes.Status200OK)]
        [ProducesResponseType(StatusCodes.Status400BadRequest)]
        [Produces("application/json")]
        public async Task<ActionResult<Instance>> UpdateReadStatus(
          [FromRoute] int instanceOwnerPartyId,
          [FromRoute] Guid instanceGuid,
          [FromQuery] string status)
        {
            if (!Enum.TryParse(status, true, out ReadStatus newStatus))
            {
                return BadRequest($"Invalid read status: {status}. Accepted types include: {string.Join(", ", Enum.GetNames(typeof(ReadStatus)))}");
            }

            Instance instance = await _instanceRepository.GetOne(instanceOwnerPartyId, instanceGuid);

            Instance updatedInstance;
            try
            {
                if (instance.Status == null)
                {
                    instance.Status = new InstanceStatus();
                }

                instance.Status.ReadStatus = newStatus;

                updatedInstance = await _instanceRepository.Update(instance);
                updatedInstance.SetPlatformSelfLinks(_storageBaseAndHost);
            }
            catch (Exception e)
            {
                _logger.LogError(e, "Unable to update read status for instance {instanceOwnerPartyId}/{instanceGuid}", instanceOwnerPartyId, instanceGuid);
                return StatusCode(StatusCodes.Status500InternalServerError);
            }

            return Ok(updatedInstance);
        }

        /// <summary>
        /// Update instance sub status.
        /// </summary>
        /// <param name="instanceOwnerPartyId">The party id of the instance owner.</param>
        /// <param name="instanceGuid">The id of the instance to confirm as complete.</param>
        /// <param name="substatus">The updated sub status.</param>
        /// <returns>Returns the updated instance.</returns>
        [Authorize]
        [HttpPut("{instanceOwnerPartyId:int}/{instanceGuid:guid}/substatus")]
        [ProducesResponseType(StatusCodes.Status200OK)]
        [ProducesResponseType(StatusCodes.Status400BadRequest)]
        [Produces("application/json")]
        public async Task<ActionResult<Instance>> UpdateSubstatus(
          [FromRoute] int instanceOwnerPartyId,
          [FromRoute] Guid instanceGuid,
          [FromBody] Substatus substatus)
        {
            DateTime creationTime = DateTime.UtcNow;

            if (substatus == null || string.IsNullOrEmpty(substatus.Label))
            {
                return BadRequest($"Invalid sub status: {JsonConvert.SerializeObject(substatus)}. Substatus must be defined and include a label.");
            }

            Instance instance = await _instanceRepository.GetOne(instanceOwnerPartyId, instanceGuid);

            string org = User.GetOrg();
            if (!instance.Org.Equals(org))
            {
                return Forbid();
            }

            Instance updatedInstance;
            try
            {
                if (instance.Status == null)
                {
                    instance.Status = new InstanceStatus();
                }

                instance.Status.Substatus = substatus;
                instance.LastChanged = creationTime;
                instance.LastChangedBy = User.GetOrgNumber();

                updatedInstance = await _instanceRepository.Update(instance);
                updatedInstance.SetPlatformSelfLinks(_storageBaseAndHost);
            }
            catch (Exception e)
            {
                _logger.LogError(e, "Unable to update sub status for instance {instaneOwnerPartyId}/{instanceGuid}", instanceOwnerPartyId, instanceGuid);
                return StatusCode(StatusCodes.Status500InternalServerError);
            }

            await DispatchEvent(InstanceEventType.SubstatusUpdated, updatedInstance);
            return Ok(updatedInstance);
        }

        /// <summary>
        /// Updates the presentation texts on an instance
        /// </summary>
        /// <param name="instanceOwnerPartyId">The party id of the instance owner.</param>
        /// <param name="instanceGuid">The id of the instance to confirm as complete.</param>
        /// <param name="presentationTexts">Collection of changes to the presentation texts collection.</param>
        /// <returns>The instance that was updated with an updated collection of presentation texts.</returns>
        [Authorize(Policy = AuthzConstants.POLICY_INSTANCE_WRITE)]
        [HttpPut("{instanceOwnerPartyId:int}/{instanceGuid:guid}/presentationtexts")]
        [ProducesResponseType(StatusCodes.Status200OK)]
        [Consumes("application/json")]
        [Produces("application/json")]
        public async Task<ActionResult<Instance>> UpdatePresentationTexts(
            [FromRoute] int instanceOwnerPartyId,
            [FromRoute] Guid instanceGuid,
            [FromBody] PresentationTexts presentationTexts)
        {
            if (presentationTexts?.Texts == null)
            {
                return BadRequest($"Missing parameter value: presentationTexts is misformed or empty");
            }

            Instance instance = await _instanceRepository.GetOne(instanceOwnerPartyId, instanceGuid);

            if (instance.PresentationTexts == null)
            {
                instance.PresentationTexts = new Dictionary<string, string>();
            }

            foreach (KeyValuePair<string, string> entry in presentationTexts.Texts)
            {
                if (string.IsNullOrEmpty(entry.Value))
                {
                    instance.PresentationTexts.Remove(entry.Key);
                }
                else
                {
                    instance.PresentationTexts[entry.Key] = entry.Value;
                }
            }

            Instance updatedInstance = await _instanceRepository.Update(instance);
            return updatedInstance;
        }

        /// <summary>
        /// Updates the data values on an instance.
        /// </summary>
        /// <param name="instanceOwnerPartyId">The party id of the instance owner.</param>
        /// <param name="instanceGuid">The id of the instance to confirm as complete.</param>
        /// <param name="dataValues">Collection of changes to the presentation texts collection.</param>
        /// <returns>The instance that was updated with an updated collection of data values.</returns>
        [Authorize(Policy = AuthzConstants.POLICY_INSTANCE_WRITE)]
        [HttpPut("{instanceOwnerPartyId:int}/{instanceGuid:guid}/datavalues")]
        [ProducesResponseType(StatusCodes.Status200OK)]
        [ProducesResponseType(StatusCodes.Status400BadRequest)]
        [Consumes("application/json")]
        [Produces("application/json")]
        public async Task<ActionResult<Instance>> UpdateDataValues(
            [FromRoute] int instanceOwnerPartyId,
            [FromRoute] Guid instanceGuid,
            [FromBody] DataValues dataValues)
        {
            if (dataValues?.Values == null)
            {
                return BadRequest($"Missing parameter value: dataValues is misformed or empty");
            }

            Instance instance = await _instanceRepository.GetOne(instanceOwnerPartyId, instanceGuid);

            instance.DataValues ??= new Dictionary<string, string>();

            foreach (KeyValuePair<string, string> entry in dataValues.Values)
            {
                if (string.IsNullOrEmpty(entry.Value))
                {
                    instance.DataValues.Remove(entry.Key);
                }
                else
                {
                    instance.DataValues[entry.Key] = entry.Value;
                }
            }

            var updatedInstance = await _instanceRepository.Update(instance);
            return Ok(updatedInstance);
        }

        private static Instance CreateInstanceFromTemplate(Application appInfo, Instance instanceTemplate, DateTime creationTime, string performedBy)
        {
            Instance createdInstance = new Instance
            {
                InstanceOwner = instanceTemplate.InstanceOwner,
                CreatedBy = performedBy,
                Created = creationTime,
                LastChangedBy = performedBy,
                LastChanged = creationTime,
                AppId = appInfo.Id,
                Org = appInfo.Org,
                VisibleAfter = DateTimeHelper.ConvertToUniversalTime(instanceTemplate.VisibleAfter) ?? creationTime,
                Status = instanceTemplate.Status ?? new InstanceStatus(),
                DueBefore = DateTimeHelper.ConvertToUniversalTime(instanceTemplate.DueBefore),
                Data = new List<DataElement>(),
                Process = instanceTemplate.Process,
                DataValues = instanceTemplate.DataValues,
            };

            return createdInstance;
        }

        private async Task<(Application Application, ActionResult ErrorMessage)> GetApplicationOrErrorAsync(string appId)
        {
            ActionResult errorResult = null;
            Application appInfo = null;

            try
            {
                string org = appId.Split("/")[0];

                appInfo = await _applicationRepository.FindOne(appId, org);

                if (appInfo == null)
                {
                    errorResult = NotFound($"Did not find application with appId={appId}");
                }
            }
            catch (Exception e)
            {
                errorResult = StatusCode(500, $"Unable to perform request: {e}");
            }

            return (appInfo, errorResult);
        }

        private async Task DispatchEvent(InstanceEventType eventType, Instance instance)
        {
            InstanceEvent instanceEvent = new InstanceEvent
            {
                EventType = eventType.ToString(),
                InstanceId = instance.Id,
                InstanceOwnerPartyId = instance.InstanceOwner.PartyId,
                User = new PlatformUser
                {
                    UserId = User.GetUserId(),
                    AuthenticationLevel = User.GetAuthenticationLevel(),
                    OrgId = User.GetOrg(),
                    SystemUserId = User.GetSystemUserId(),
                    SystemUserOwnerOrgNo = User.GetSystemUserOwner(),
                },

                ProcessInfo = instance.Process,
                Created = DateTime.UtcNow,
            };

            await _instanceEventRepository.InsertInstanceEvent(instanceEvent);
        }

        private static string BuildQueryStringWithOneReplacedParameter(Dictionary<string, StringValues> q, string queryParamName, string newParamValue)
        {
            List<KeyValuePair<string, string>> items = q.SelectMany(
                x => x.Value,
                (col, value) => new KeyValuePair<string, string>(col.Key, value))
                .ToList();

            items.RemoveAll(x => x.Key == queryParamName);

            var qb = new QueryBuilder(items)
                        {
                            { queryParamName, newParamValue }
                        };

            string nextQueryString = qb.ToQueryString().Value;

            return nextQueryString;
        }

        private static void FilterOutDeletedDataElements(Instance instance)
        {
            if (instance.Data != null)
            {
                instance.Data = instance.Data.Where(d => d.DeleteStatus?.IsHardDeleted != true).ToList();
            }
        }

        private string BuildRequestLink(string continuationToken = null, Dictionary<string, StringValues> queryParams = null)
        {
            string host = $"https://platform.{_generalSettings.Hostname}";
            string url = Request.Path;
            string query = Request.QueryString.Value;

            if (continuationToken == null)
            {
                string selfUrl = $"{host}{url}{query}";
                return selfUrl;
            }
            else
            {
                string selfQueryString = BuildQueryStringWithOneReplacedParameter(
                    queryParams,
                    "continuationToken",
                    continuationToken);

                string selfUrl = $"{host}{url}{selfQueryString}";

                return selfUrl;
            }
        }
    }
}

﻿#nullable enable
#if !LOCALTEST
using Altinn.Notifications.Configuration;
#endif
using Altinn.Notifications.Core.Models;
using Altinn.Notifications.Core.Models.Orders;
using Altinn.Notifications.Core.Services.Interfaces;
using Altinn.Notifications.Extensions;
using Altinn.Notifications.Mappers;
using Altinn.Notifications.Models;
using Altinn.Notifications.Validators;

using FluentValidation;

#if !LOCALTEST
using Microsoft.AspNetCore.Authorization;
#endif
using Microsoft.AspNetCore.Mvc;

#if !LOCALTEST
using Swashbuckle.AspNetCore.Annotations;
using Swashbuckle.AspNetCore.Filters;
#endif

namespace Altinn.Notifications.Controllers;

/// <summary>
/// Controller for all operations related to email notification orders
/// </summary>
[Route("notifications/api/v1/orders/email")]
[ApiController]
#if !LOCALTEST
[Authorize(Policy = AuthorizationConstants.POLICY_CREATE_SCOPE_OR_PLATFORM_ACCESS)]
[SwaggerResponse(401, "Caller is unauthorized")]
[SwaggerResponse(403, "Caller is not authorized to access the requested resource")]
# endif
public class EmailNotificationOrdersController : ControllerBase
{
    private readonly IValidator<EmailNotificationOrderRequestExt> _validator;
    private readonly IEmailNotificationOrderService _orderService;

    /// <summary>
    /// Initializes a new instance of the <see cref="EmailNotificationOrdersController"/> class.
    /// </summary>
    public EmailNotificationOrdersController(IValidator<EmailNotificationOrderRequestExt> validator, IEmailNotificationOrderService orderService)
    {
        _validator = validator;
        _orderService = orderService;
    }

    /// <summary>
    /// Add an email notification order.
    /// </summary>
    /// <remarks>
    /// The API will accept the request after som basic validation of the request.
    /// The system will also attempt to verify that it will be possible to fulfill the order.
    /// </remarks>
    /// <returns>The id of the registered notification order</returns>
    [HttpPost]
    [Consumes("application/json")]
    [Produces("application/json")]
#if !LOCALTEST
    [SwaggerResponse(202, "The notification order was accepted", typeof(OrderIdExt))]
    [SwaggerResponse(400, "The notification order is invalid", typeof(ValidationProblemDetails))]
    [SwaggerResponseHeader(202, "Location", "string", "Link to access the newly created notification order.")]
#endif
    public async Task<ActionResult<OrderIdExt>> Post(EmailNotificationOrderRequestExt emailNotificationOrderRequest)
    {
        var validationResult = _validator.Validate(emailNotificationOrderRequest);
        if (!validationResult.IsValid)
        {
            validationResult.AddToModelState(ModelState);
            return ValidationProblem(ModelState);
        }

#if LOCALTEST
        string creator = "localtest";
#else
        string? creator = HttpContext.GetOrg();

        if (creator == null)
        {
            return Forbid();
        }
#endif

        var orderRequest = emailNotificationOrderRequest.MapToOrderRequest(creator);
        (NotificationOrder? registeredOrder, ServiceError? error) = await _orderService.RegisterEmailNotificationOrder(orderRequest);

        if (error != null)
        {
            return StatusCode(error.ErrorCode, error.ErrorMessage);
        }

        string selfLink = registeredOrder!.GetSelfLink();
        return Accepted(selfLink, new OrderIdExt(registeredOrder!.Id));
    }
}
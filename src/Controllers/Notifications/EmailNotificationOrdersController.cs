using Altinn.Notifications.Core.Models.Orders;
using Altinn.Notifications.Core.Services.Interfaces;
using Altinn.Notifications.Extensions;
using Altinn.Notifications.Mappers;
using Altinn.Notifications.Models;
using Altinn.Notifications.Validators;

using FluentValidation;

using LocalTest.Models;

using Microsoft.AspNetCore.Mvc;

namespace Altinn.Notifications.Controllers;

/// <summary>
/// Controller for all operations related to email notification orders
/// </summary>
[Route("notifications/api/v1/orders/email")]
[ApiController]
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
    public async Task<ActionResult<OrderIdExt>> Post(EmailNotificationOrderRequestExt emailNotificationOrderRequest)
    {
        var validationResult = _validator.Validate(emailNotificationOrderRequest);
        if (!validationResult.IsValid)
        {
            validationResult.AddToModelState(this.ModelState);
            return ValidationProblem(ModelState);
        }

        string creator = "localtest";

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
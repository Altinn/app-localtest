using Altinn.Notifications.Core.Models;

using LocalTest.Notifications.Core.Models.Orders;

namespace LocalTest.Notifications.Core.Services;

/// <summary>
/// Interface for the email notification order service
/// </summary>
public interface IEmailNotificationOrderService
{
    /// <summary>
    /// Registers a new order
    /// </summary>
    /// <param name="orderRequest">The email notification order request</param>
    /// <returns>The registered notification order</returns>
    public Task<(NotificationOrder Order, ServiceError Error)> RegisterEmailNotificationOrder(NotificationOrderRequest orderRequest);
}

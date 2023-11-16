using Altinn.Notifications.Core.Models;

using LocalTest.Models;
using LocalTest.Notifications.Core.Models.NotificationTemplate;
using LocalTest.Notifications.Core.Models.Orders;
using LocalTest.Notifications.Core.Repository;

namespace LocalTest.Notifications.Core.Services;

/// <summary>
/// Implementation of the <see cref="IEmailNotificationOrderService"/>. 
/// </summary>
public class EmailNotificationOrderService : IEmailNotificationOrderService
{
    private readonly IOrderRepository _repository;
    private readonly IGuidService _guid;
    private readonly IDateTimeService _dateTime;
    private readonly string _defaultFromAddress;

    /// <summary>
    /// Initializes a new instance of the <see cref="EmailNotificationOrderService"/> class.
    /// </summary>
    public EmailNotificationOrderService(IOrderRepository repository, IGuidService guid, IDateTimeService dateTime)
    {
        _repository = repository;
        _guid = guid;
        _dateTime = dateTime;
        _defaultFromAddress = "localtest@altinn.no";
    }

    /// <inheritdoc/>
    public async Task<(NotificationOrder Order, ServiceError Error)> RegisterEmailNotificationOrder(NotificationOrderRequest orderRequest)
    {
        Guid orderId = _guid.NewGuid();
        DateTime created = _dateTime.UtcNow();

        var templates = SetFromAddressIfNotDefined(orderRequest.Templates);

        var order = new NotificationOrder(
            orderId,
            orderRequest.SendersReference,
            templates,
            orderRequest.RequestedSendTime,
            orderRequest.NotificationChannel,
            orderRequest.Creator,
            created,
            orderRequest.Recipients);

        NotificationOrder savedOrder = await _repository.Create(order);

        return (savedOrder, null);
    }

    private List<INotificationTemplate> SetFromAddressIfNotDefined(List<INotificationTemplate> templates)
    {
        foreach (var template in templates.OfType<EmailTemplate>().Where(template => string.IsNullOrEmpty(template.FromAddress)))
        {
            template.FromAddress = _defaultFromAddress;
        }

        return templates;
    }
}

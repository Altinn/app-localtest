using Altinn.Notifications.Models;
using LocalTest.Notifications.Core.Models.Orders;

namespace Altinn.Notifications.Mappers;

/// <summary>
/// Mapper class
/// </summary>
public static class NotificationOrderRequestResponseMapper
{
    /// <summary>
    /// Maps a <see cref="NotificationOrderRequestResponse"/> to a <see cref="NotificationOrderRequestResponseExt"/>
    /// </summary>
    public static NotificationOrderRequestResponseExt MapToExternal(this NotificationOrderRequestResponse requestResponse)
    {
        NotificationOrderRequestResponseExt ext = new()
        {
            OrderId = requestResponse.OrderId
        };

        if (requestResponse.RecipientLookup != null)
        {
            ext.RecipientLookup = new RecipientLookupResultExt
            {
                Status = Enum.Parse<RecipientLookupStatusExt>(requestResponse.RecipientLookup.Status.ToString(), true),
                IsReserved = requestResponse.RecipientLookup?.IsReserved,
                MissingContact = requestResponse.RecipientLookup?.MissingContact,
            };
        }

        return ext;
    }
}

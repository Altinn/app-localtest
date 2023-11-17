﻿#nullable enable
using Altinn.Notifications.Core.Enums;

namespace Altinn.Notifications.Core.Models.Orders;

/// <summary>
/// Class representing the base properties of a notification order
/// </summary>
public interface IBaseNotificationOrder
{
    /// <summary>
    /// Gets the id of the notification order
    /// </summary>
    public Guid Id { get; }

    /// <summary>
    /// Gets the senders reference of a notification
    /// </summary>
    public string? SendersReference { get; }

    /// <summary>
    /// Gets the requested send time for the notification(s)
    /// </summary>
    public DateTime RequestedSendTime { get; }

    /// <summary>
    /// Gets the preferred notification channel
    /// </summary>
    public NotificationChannel NotificationChannel { get; }

    /// <summary>
    /// Gets the creator of the notification
    /// </summary>
    public Creator Creator { get; }

    /// <summary>
    /// Gets the date and time for when the notification order was created
    /// </summary>
    public DateTime Created { get; }
}

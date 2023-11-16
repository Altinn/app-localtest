using System.Text.Json.Serialization;
using LocalTest.Notifications.Core.Models.Enums;

namespace LocalTest.Notifications.Core.Models.NotificationTemplate;

/// <summary>
/// Base class for a notification template
/// </summary>
[JsonDerivedType(typeof(EmailTemplate), "email")]
[JsonPolymorphic(TypeDiscriminatorPropertyName = "$")]
public interface INotificationTemplate
{
    /// <summary>
    /// Gets the type for the template
    /// </summary>
    public NotificationTemplateType Type { get; }
}

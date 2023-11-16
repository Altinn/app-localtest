using System.Text.Json.Serialization;

namespace LocalTest.Notifications.API.Models;

/// <summary>
/// A class representing a set of resource links of a notification 
/// </summary>
/// <remarks>
/// External representaion to be used in the API.
/// </remarks>
public class NotificationResourceLinksExt
{
    /// <summary>
    /// Gets or sets the self link 
    /// </summary>
    [JsonPropertyName("self")]
    public string Self { get; set; } = string.Empty;
}

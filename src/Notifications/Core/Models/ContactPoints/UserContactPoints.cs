namespace Altinn.Notifications.Core.Models.ContactPoints;

/// <summary>
/// Class describing the availability of contact points for a user
/// </summary>
public class UserContactPoints
{
    /// <summary>
    /// Gets or sets the ID of the user
    /// </summary>
    public int UserId { get; set; }

    /// <summary>
    /// Gets or sets the national identityt number of the user
    /// </summary>
    public string NationalIdentityNumber { get; set; } = string.Empty;

    /// <summary>
    /// Gets or sets a boolean indicating whether the user has reserved themselves from electronic communication
    /// </summary>
    public bool IsReserved { get; set; }

    /// <summary>
    /// Gets or sets the mobile number
    /// </summary>
    public string MobileNumber { get; set; } = string.Empty;

    /// <summary>
    /// Gets or sets the email address
    /// </summary>
    public string Email { get; set; } = string.Empty;
}

﻿using System.Text.Json.Serialization;

namespace Altinn.Notifications.Core.Models.Notification
{
    /// <summary>
    /// A class representing an sms notification summary 
    /// </summary>
    /// <remarks>
    /// External representaion to be used in the API.
    /// </remarks>
    public class SmsNotificationSummaryExt
    {
        /// <summary>
        /// The order id
        /// </summary>
        [JsonPropertyName("orderId")]
        public Guid OrderId { get; set; }

        /// <summary>
        /// The senders reference
        /// </summary>
        [JsonPropertyName("sendersReference")]
        public string? SendersReference { get; set; }

        /// <summary>
        /// The number of generated sms notifications
        /// </summary>
        [JsonPropertyName("generated")]
        public int Generated { get; set; }

        /// <summary>
        /// The number of sms notifications that were sent successfully
        /// </summary>
        [JsonPropertyName("succeeded")]
        public int Succeeded { get; set; }

        /// <summary>
        /// A list of notifications with send result 
        /// </summary>
        [JsonPropertyName("notifications")]
        public List<SmsNotificationWithResultExt> Notifications { get; set; } = new();
    }
}

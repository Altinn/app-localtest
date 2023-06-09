using System.Text.Json;
using System.Text.Json.Serialization;

using Altinn.Platform.Storage.Helpers;

namespace LocalTest.Helpers
{

    public class CustomDateTimeConverter : JsonConverter<DateTime>
    {
        private string[] dateFormats = new string[] { "yyyy-MM-ddTHH:mm:ss.fffffffZ", "yyyy-MM-ddTHH.mm.ss.fffZ", "yyyy-MM-ddTHH.mm.ss.ffZ" };

        public override void Write(Utf8JsonWriter writer, DateTime date, JsonSerializerOptions options)
        {
            writer.WriteStringValue(date.ToString(DateTimeHelper.Iso8601UtcFormat));
        }
        public override DateTime Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options)
        {
            return DateTime.ParseExact(reader.GetString(), dateFormats, null);
        }
    }
}

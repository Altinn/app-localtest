using System.Text.Json;

using LocalTest.Configuration;
using LocalTest.Notifications.Core.Models.Enums;
using LocalTest.Notifications.Core.Models.Orders;
using LocalTest.Notifications.Core.Repository;
using Microsoft.Extensions.Options;

namespace LocalTest.Notifications.Persistence.Repository
{
    public class OrderRepository : IOrderRepository
    {
        private readonly LocalPlatformSettings _localPlatformSettings;
        private readonly JsonSerializerOptions _serializerOptions;

        public OrderRepository(
            IOptions<LocalPlatformSettings> localPlatformSettings)
        {
            _localPlatformSettings = localPlatformSettings.Value;
            Directory.CreateDirectory(GetNotificationsDbPath());

            _serializerOptions = new JsonSerializerOptions
            {
                WriteIndented = true,
                PropertyNamingPolicy = JsonNamingPolicy.CamelCase
            };
        }

        public Task<NotificationOrder> Create(NotificationOrder order)
        {
            string path = GetOrderPath(order.Id);

            string serializedOrder = JsonSerializer.Serialize(order, _serializerOptions);
            FileInfo file = new FileInfo(path);
            file.Directory.Create();
            File.WriteAllText(file.FullName, serializedOrder);

            return Task.FromResult(order);
        }

        public Task<NotificationOrder> GetOrderById(Guid id, string creator)
        {
            throw new NotImplementedException();
        }

        public Task<List<NotificationOrder>> GetOrdersBySendersReference(string sendersReference, string creator)
        {
            throw new NotImplementedException();
        }

        public Task<NotificationOrderWithStatus> GetOrderWithStatusById(Guid id, string creator)
        {
            throw new NotImplementedException();
        }

        public Task<List<NotificationOrder>> GetPastDueOrdersAndSetProcessingState()
        {
            throw new NotImplementedException();
        }

        public Task SetProcessingStatus(Guid orderId, OrderProcessingStatus status)
        {
            throw new NotImplementedException();
        }

        private string GetOrderPath(Guid orderId)
        {
            return Path.Combine(GetNotificationsDbPath(), "orders", orderId + ".json");
        }

        private string GetNotificationsDbPath()
        {
            return _localPlatformSettings.LocalTestingStorageBasePath + _localPlatformSettings.NotificationsStorageFolder;
        }

    }
}

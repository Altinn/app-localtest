FROM mcr.microsoft.com/dotnet/sdk:9.0-alpine@sha256:6fc61b57f8fa5f333e30c4192076fbd8b76f90dd2c5b2e5f9066f76f5b726832 AS build
WORKDIR /src

COPY ./src/LocalTest.csproj .
RUN dotnet restore LocalTest.csproj

COPY ./src .
RUN dotnet publish LocalTest.csproj -c Release -o /app_output

FROM mcr.microsoft.com/dotnet/aspnet:8.0-alpine@sha256:0389d5b7d60f75ebbeec3bfffd2ad0a06d234e7b998231a5a86abf5e919a7d01 AS final
EXPOSE 5101
WORKDIR /app
COPY --from=build /app_output .

# Copy various data
COPY ./testdata /testdata

# setup the user and group (not important for LocalTest and this removes write access to /AltinnPlatformLocal)
# RUN addgroup -g 3000 dotnet && adduser -u 1000 -G dotnet -D -s /bin/false dotnet
# USER dotnet

ENTRYPOINT ["dotnet", "LocalTest.dll"]

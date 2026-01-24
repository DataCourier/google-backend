package com.example.buckets

import io.ktor.client.*
import io.ktor.client.call.*
import io.ktor.client.plugins.contentnegotiation.*
import io.ktor.client.request.*
import io.ktor.client.statement.*
import io.ktor.http.*
import io.ktor.serialization.kotlinx.json.*
import kotlinx.serialization.json.*

/**
 * HTTP client for bucket API.
 */
class BucketClient(
    private val baseUrl: String,
    private val httpClient: HttpClient = defaultHttpClient()
) {
    private var authToken: String? = null

    companion object {
        fun defaultHttpClient() = HttpClient {
            install(ContentNegotiation) {
                json(Json {
                    ignoreUnknownKeys = true
                    isLenient = true
                })
            }
        }
    }

    /** Set the auth token (session token from magic link or local:xxx for dev). */
    fun setAuthToken(token: String) {
        authToken = token
    }

    /** Clear auth token (logout). */
    fun clearAuthToken() {
        authToken = null
    }

    // =========================================================================
    // Auth endpoints
    // =========================================================================

    /** Request a magic link for email. */
    suspend fun requestMagicLink(email: String): Result<String> = runCatching {
        val response = httpClient.post("$baseUrl/auth/magic-link") {
            contentType(ContentType.Application.Json)
            setBody(mapOf("email" to email))
        }
        checkResponse(response)
        "Check your email for the login link"
    }

    /** Verify magic link token and get session. */
    suspend fun verifyMagicLink(token: String): Result<AuthSession> = runCatching {
        val response = httpClient.get("$baseUrl/auth/verify") {
            parameter("token", token)
        }
        checkResponse(response)
        val json = response.body<JsonObject>()
        AuthSession(
            token = json["token"]?.jsonPrimitive?.content ?: error("missing token"),
            userId = json["user_id"]?.jsonPrimitive?.content ?: error("missing user_id"),
            email = json["email"]?.jsonPrimitive?.content ?: error("missing email")
        )
    }

    /** Logout (revoke session). */
    suspend fun logout(): Result<Unit> = runCatching {
        val response = httpClient.post("$baseUrl/auth/logout") {
            authToken?.let { header("Authorization", "Bearer $it") }
        }
        checkResponse(response)
        clearAuthToken()
    }

    // =========================================================================
    // Personal bucket endpoints
    // =========================================================================

    suspend fun createPersonal(bucket: String, data: JsonObject): Result<JsonObject> = runCatching {
        val response = httpClient.post("$baseUrl/buckets/mine/$bucket") {
            contentType(ContentType.Application.Json)
            authToken?.let { header("Authorization", it) }
            setBody(data)
        }
        checkResponse(response)
        response.body<JsonObject>()["data"]?.jsonObject ?: error("missing data")
    }

    suspend fun getPersonal(bucket: String, id: String): Result<JsonObject> = runCatching {
        val response = httpClient.get("$baseUrl/buckets/mine/$bucket/$id") {
            authToken?.let { header("Authorization", it) }
        }
        checkResponse(response)
        response.body<JsonObject>()["data"]?.jsonObject ?: error("missing data")
    }

    suspend fun updatePersonal(bucket: String, id: String, data: JsonObject): Result<JsonObject> = runCatching {
        val response = httpClient.put("$baseUrl/buckets/mine/$bucket/$id") {
            contentType(ContentType.Application.Json)
            authToken?.let { header("Authorization", it) }
            setBody(data)
        }
        checkResponse(response)
        response.body<JsonObject>()["data"]?.jsonObject ?: error("missing data")
    }

    suspend fun deletePersonal(bucket: String, id: String): Result<Unit> = runCatching {
        val response = httpClient.delete("$baseUrl/buckets/mine/$bucket/$id") {
            authToken?.let { header("Authorization", it) }
        }
        checkResponse(response)
    }

    suspend fun listPersonal(bucket: String): Result<List<JsonObject>> = runCatching {
        val response = httpClient.get("$baseUrl/buckets/mine/$bucket") {
            authToken?.let { header("Authorization", it) }
        }
        checkResponse(response)
        response.body<JsonObject>()["data"]?.jsonArray?.map { it.jsonObject } ?: emptyList()
    }

    // =========================================================================
    // Org bucket endpoints
    // =========================================================================

    suspend fun createOrg(orgId: String, bucket: String, data: JsonObject): Result<JsonObject> = runCatching {
        val response = httpClient.post("$baseUrl/org/$orgId/buckets/$bucket/") {
            contentType(ContentType.Application.Json)
            authToken?.let { header("Authorization", it) }
            setBody(data)
        }
        checkResponse(response)
        response.body<JsonObject>()["data"]?.jsonObject ?: error("missing data")
    }

    suspend fun getOrg(orgId: String, bucket: String, id: String): Result<JsonObject> = runCatching {
        val response = httpClient.get("$baseUrl/org/$orgId/buckets/$bucket/$id") {
            authToken?.let { header("Authorization", it) }
        }
        checkResponse(response)
        response.body<JsonObject>()["data"]?.jsonObject ?: error("missing data")
    }

    suspend fun listOrg(orgId: String, bucket: String): Result<List<JsonObject>> = runCatching {
        val response = httpClient.get("$baseUrl/org/$orgId/buckets/$bucket/") {
            authToken?.let { header("Authorization", it) }
        }
        checkResponse(response)
        response.body<JsonObject>()["data"]?.jsonArray?.map { it.jsonObject } ?: emptyList()
    }

    suspend fun updateOrg(orgId: String, bucket: String, id: String, data: JsonObject): Result<JsonObject> = runCatching {
        val response = httpClient.put("$baseUrl/org/$orgId/buckets/$bucket/$id") {
            contentType(ContentType.Application.Json)
            authToken?.let { header("Authorization", it) }
            setBody(data)
        }
        checkResponse(response)
        response.body<JsonObject>()["data"]?.jsonObject ?: error("missing data")
    }

    suspend fun deleteOrg(orgId: String, bucket: String, id: String): Result<Unit> = runCatching {
        val response = httpClient.delete("$baseUrl/org/$orgId/buckets/$bucket/$id") {
            authToken?.let { header("Authorization", it) }
        }
        checkResponse(response)
    }

    // =========================================================================
    // Helpers
    // =========================================================================

    private suspend fun checkResponse(response: HttpResponse) {
        if (!response.status.isSuccess()) {
            val error = try {
                response.body<JsonObject>()["error"]?.jsonPrimitive?.content
            } catch (e: Exception) {
                null
            }
            throw BucketException(response.status.value, error ?: response.status.description)
        }
    }
}

data class AuthSession(
    val token: String,
    val userId: String,
    val email: String
)

class BucketException(val statusCode: Int, message: String) : Exception(message)

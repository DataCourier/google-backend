package com.example.buckets

// =============================================================================
// Example Models with Relationships
// =============================================================================

/**
 * A blog post with comments.
 */
@PersonalBucket("posts")
class Post(
    var title: String = "",
    var content: String = "",
    var published: Boolean = false
) : PersonalActiveRecord() {

    /** Has many comments - foreign key is "postId" on Comment */
    val comments by hasMany<Comment>()

    /** Has many tags */
    val tags by hasMany<Tag>()

    companion object {
        fun find(id: String): Post? = ActiveRecord.find(id)
        fun all(): List<Post> = ActiveRecord.all()
        fun where(predicate: (Post) -> Boolean): List<Post> = ActiveRecord.where(predicate)
    }
}

/**
 * A comment on a post.
 */
@PersonalBucket("comments")
class Comment(
    var text: String = "",
    var authorName: String = ""
) : PersonalActiveRecord() {

    /** Foreign key to Post */
    @BelongsTo(Post::class)
    var postId: String = ""

    /** Get the parent post */
    val post by belongsTo<Post>(::postId)

    companion object {
        fun find(id: String): Comment? = ActiveRecord.find(id)
        fun all(): List<Comment> = ActiveRecord.all()
    }
}

/**
 * A tag on a post.
 */
@PersonalBucket("tags")
class Tag(
    var name: String = ""
) : PersonalActiveRecord() {

    @BelongsTo(Post::class)
    var postId: String = ""

    val post by belongsTo<Post>(::postId)

    companion object {
        fun find(id: String): Tag? = ActiveRecord.find(id)
        fun all(): List<Tag> = ActiveRecord.all()
    }
}

/**
 * User with a profile (has-one).
 */
@PersonalBucket("app-users")
class AppUser(
    var username: String = "",
    var email: String = ""
) : PersonalActiveRecord() {

    /** Has one profile */
    val profile by hasOne<UserProfile>()

    /** Has many posts */
    val posts by hasMany<UserPost>("authorId")

    companion object {
        fun find(id: String): AppUser? = ActiveRecord.find(id)
        fun all(): List<AppUser> = ActiveRecord.all()
    }
}

@PersonalBucket("user-profiles")
class UserProfile(
    var bio: String = "",
    var avatarUrl: String = "",
    var location: String = ""
) : PersonalActiveRecord() {

    @BelongsTo(AppUser::class)
    var appUserId: String = ""

    val user by belongsTo<AppUser>(::appUserId)

    companion object {
        fun find(id: String): UserProfile? = ActiveRecord.find(id)
    }
}

@PersonalBucket("user-posts")
class UserPost(
    var title: String = ""
) : PersonalActiveRecord() {

    @BelongsTo(AppUser::class)
    var authorId: String = ""

    val author by belongsTo<AppUser>(::authorId)

    companion object {
        fun find(id: String): UserPost? = ActiveRecord.find(id)
        fun all(): List<UserPost> = ActiveRecord.all()
    }
}

// =============================================================================
// Usage Examples
// =============================================================================

fun relationshipExamples() {
    BucketContext.configure {
        baseUrl = "http://localhost:8080"
        userId = "local:demo-user"
        storage = InMemoryStorage()
    }

    // -------------------------------------------------------------------------
    // HasMany: Post -> Comments
    // -------------------------------------------------------------------------

    // Create a post
    val post = Post(title = "Hello World", content = "My first post!")
    post.save()
    println("Created post: ${post.id}")

    // Create comments via the relationship (postId auto-filled!)
    val comment1 = post.comments.build().apply {
        text = "Great post!"
        authorName = "Alice"
    }
    comment1.save()

    val comment2 = post.comments.build().apply {
        text = "Thanks for sharing"
        authorName = "Bob"
    }
    comment2.save()

    // Query comments for this post
    println("Post has ${post.comments.count()} comments:")
    post.comments.all().forEach { comment ->
        println("  - ${comment.authorName}: ${comment.text}")
    }

    // Filter comments
    val aliceComments = post.comments.where { it.authorName == "Alice" }
    println("Alice's comments: ${aliceComments.size}")

    // -------------------------------------------------------------------------
    // BelongsTo: Comment -> Post
    // -------------------------------------------------------------------------

    // Navigate from comment back to post
    val loadedComment = Comment.find(comment1.id)
    val parentPost = loadedComment?.post
    println("Comment belongs to post: ${parentPost?.title}")

    // -------------------------------------------------------------------------
    // HasOne: User -> Profile
    // -------------------------------------------------------------------------

    val user = AppUser(username = "johndoe", email = "john@example.com")
    user.save()

    // Create profile via relationship (appUserId auto-filled!)
    val profile = user.profile.build().apply {
        bio = "Software developer"
        location = "San Francisco"
    }
    profile.save()

    // Access profile
    val userProfile = user.profile.get()
    println("${user.username}'s bio: ${userProfile?.bio}")

    // Navigate back
    println("Profile belongs to: ${userProfile?.user?.username}")

    // -------------------------------------------------------------------------
    // HasMany with explicit foreign key
    // -------------------------------------------------------------------------

    // Create posts for user (using explicit "authorId" foreign key)
    val userPost = user.posts.build().apply {
        title = "My Journey"
    }
    userPost.save()

    println("${user.username} has ${user.posts.count()} posts")

    // -------------------------------------------------------------------------
    // Chained relationships
    // -------------------------------------------------------------------------

    // Add tags to post
    val tag1 = post.tags.build().apply { name = "kotlin" }
    tag1.save()

    val tag2 = post.tags.build().apply { name = "android" }
    tag2.save()

    println("Post tags: ${post.tags.all().map { it.name }}")
}

// =============================================================================
// Rails-like syntax examples
// =============================================================================

fun railsLikeSyntax() {
    BucketContext.configure {
        baseUrl = "http://localhost:8080"
        userId = "local:rails-user"
        storage = InMemoryStorage()
    }

    // Create parent
    val post = Post(title = "Rails-like Syntax")
    post.save()

    // This is very Rails-like!
    // In Rails:  post.comments.create(text: "Hello")
    // In Kotlin: post.comments.build().apply { text = "Hello" }.also { it.save() }

    // Or with a helper extension:
    post.comments.build().apply {
        text = "First!"
        authorName = "Speed Racer"
    }.save()

    // Querying
    val recentComments = post.comments.where { it.text.contains("!") }

    // Counting
    if (post.comments.isNotEmpty()) {
        println("Post has comments!")
    }

    // Finding within scope
    val comment = post.comments.all().firstOrNull()
    val sameComment = comment?.let { post.comments.find(it.id) }
}

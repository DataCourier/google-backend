package com.example.buckets

import kotlin.test.*

/**
 * Tests for Rails-style relationships.
 */
class RelationshipsTest {

    @BeforeTest
    fun setup() {
        BucketContext.configure {
            storage = InMemoryStorage()
            userId = "test-user"
        }
    }

    @AfterTest
    fun cleanup() {
        BucketContext.storage.clearAll()
    }

    // =========================================================================
    // HasMany Tests
    // =========================================================================

    @Test
    fun `hasMany returns empty list when no children exist`() {
        val post = TestPost(title = "Hello")
        post.save()

        assertEquals(0, post.comments.count())
        assertTrue(post.comments.isEmpty())
        assertEquals(emptyList(), post.comments.all())
    }

    @Test
    fun `hasMany build auto-fills foreign key`() {
        val post = TestPost(title = "Hello")
        post.save()

        val comment = post.comments.build().apply {
            text = "Great post!"
        }

        assertEquals(post.id, comment.postId)
    }

    @Test
    fun `hasMany returns only children belonging to parent`() {
        val post1 = TestPost(title = "Post 1")
        post1.save()

        val post2 = TestPost(title = "Post 2")
        post2.save()

        // Create comments for post1
        val comment1 = post1.comments.build().apply { text = "Comment on post 1" }
        comment1.save()

        val comment2 = post1.comments.build().apply { text = "Another on post 1" }
        comment2.save()

        // Create comment for post2
        val comment3 = post2.comments.build().apply { text = "Comment on post 2" }
        comment3.save()

        // Verify counts
        assertEquals(2, post1.comments.count())
        assertEquals(1, post2.comments.count())

        // Verify correct comments returned
        val post1Comments = post1.comments.all()
        assertTrue(post1Comments.all { it.postId == post1.id })
        assertTrue(post1Comments.any { it.text == "Comment on post 1" })
        assertTrue(post1Comments.any { it.text == "Another on post 1" })

        val post2Comments = post2.comments.all()
        assertTrue(post2Comments.all { it.postId == post2.id })
        assertEquals("Comment on post 2", post2Comments.first().text)
    }

    @Test
    fun `hasMany where filters children`() {
        val post = TestPost(title = "Hello")
        post.save()

        post.comments.build().apply { text = "Great!" }.save()
        post.comments.build().apply { text = "Awesome!" }.save()
        post.comments.build().apply { text = "Meh" }.save()

        val excitedComments = post.comments.where { it.text.endsWith("!") }

        assertEquals(2, excitedComments.size)
        assertTrue(excitedComments.all { it.text.endsWith("!") })
    }

    @Test
    fun `hasMany find returns child only if belongs to parent`() {
        val post1 = TestPost(title = "Post 1")
        post1.save()

        val post2 = TestPost(title = "Post 2")
        post2.save()

        val comment = post1.comments.build().apply { text = "Comment" }
        comment.save()

        // Can find via correct parent
        assertNotNull(post1.comments.find(comment.id))

        // Cannot find via wrong parent
        assertNull(post2.comments.find(comment.id))
    }

    @Test
    fun `hasMany isNotEmpty returns true when children exist`() {
        val post = TestPost(title = "Hello")
        post.save()

        assertTrue(post.comments.isEmpty())
        assertFalse(post.comments.isNotEmpty())

        post.comments.build().apply { text = "Hi" }.save()

        assertFalse(post.comments.isEmpty())
        assertTrue(post.comments.isNotEmpty())
    }

    // =========================================================================
    // HasOne Tests
    // =========================================================================

    @Test
    fun `hasOne get returns null when no related record exists`() {
        val user = TestUser(username = "alice")
        user.save()

        assertNull(user.profile.get())
    }

    @Test
    fun `hasOne build auto-fills foreign key`() {
        val user = TestUser(username = "alice")
        user.save()

        val profile = user.profile.build().apply {
            bio = "Developer"
        }

        assertEquals(user.id, profile.userId)
    }

    @Test
    fun `hasOne get returns related record`() {
        val user = TestUser(username = "alice")
        user.save()

        val profile = user.profile.build().apply {
            bio = "Developer"
            location = "NYC"
        }
        profile.save()

        val loaded = user.profile.get()

        assertNotNull(loaded)
        assertEquals("Developer", loaded.bio)
        assertEquals("NYC", loaded.location)
        assertEquals(user.id, loaded.userId)
    }

    @Test
    fun `hasOne returns correct profile for each user`() {
        val user1 = TestUser(username = "alice")
        user1.save()
        user1.profile.build().apply { bio = "Alice's bio" }.save()

        val user2 = TestUser(username = "bob")
        user2.save()
        user2.profile.build().apply { bio = "Bob's bio" }.save()

        assertEquals("Alice's bio", user1.profile.get()?.bio)
        assertEquals("Bob's bio", user2.profile.get()?.bio)
    }

    // =========================================================================
    // BelongsTo Tests
    // =========================================================================

    @Test
    fun `belongsTo returns null when foreign key is empty`() {
        val comment = TestComment(text = "Orphan comment")
        // postId is empty by default

        assertNull(comment.post)
    }

    @Test
    fun `belongsTo returns parent record`() {
        val post = TestPost(title = "Hello World")
        post.save()

        val comment = post.comments.build().apply {
            text = "Great post!"
        }
        comment.save()

        val parent = comment.post

        assertNotNull(parent)
        assertEquals("Hello World", parent.title)
        assertEquals(post.id, parent.id)
    }

    @Test
    fun `belongsTo navigates from child to parent`() {
        val post = TestPost(title = "Original Post")
        post.save()

        val comment = TestComment(text = "My comment")
        comment.postId = post.id
        comment.save()

        // Load comment fresh and navigate to parent
        val loadedComment = TestComment.find(comment.id)
        assertNotNull(loadedComment)

        val parentPost = loadedComment.post
        assertNotNull(parentPost)
        assertEquals("Original Post", parentPost.title)
    }

    // =========================================================================
    // Integration Tests
    // =========================================================================

    @Test
    fun `full relationship workflow`() {
        // Create user with profile
        val user = TestUser(username = "alice")
        user.save()

        val profile = user.profile.build().apply {
            bio = "Software Engineer"
            location = "San Francisco"
        }
        profile.save()

        // Create posts for user (using explicit foreign key)
        val post = TestUserPost(title = "My First Post")
        post.authorId = user.id
        post.save()

        // Add comments to post
        val comment1 = post.comments.build().apply { text = "Great!" }
        comment1.save()

        val comment2 = post.comments.build().apply { text = "Love it!" }
        comment2.save()

        // Verify relationships
        assertEquals("Software Engineer", user.profile.get()?.bio)
        assertEquals(2, post.comments.count())

        // Navigate backwards
        assertEquals(post.id, comment1.post?.id)
    }

    @Test
    fun `deleted children are not returned by hasMany`() {
        val post = TestPost(title = "Hello")
        post.save()

        val comment1 = post.comments.build().apply { text = "Keep me" }
        comment1.save()

        val comment2 = post.comments.build().apply { text = "Delete me" }
        comment2.save()

        assertEquals(2, post.comments.count())

        // Delete one comment
        comment2.delete()

        assertEquals(1, post.comments.count())
        assertEquals("Keep me", post.comments.all().first().text)
    }

    @Test
    fun `explicit foreign key works with hasMany`() {
        val user = TestUser(username = "alice")
        user.save()

        // TestUserPost uses authorId (explicit foreign key)
        val post = user.posts.build().apply {
            title = "Alice's Post"
        }
        post.save()

        assertEquals(user.id, post.authorId)
        assertEquals(1, user.posts.count())
        assertEquals("Alice's Post", user.posts.all().first().title)
    }
}

// =============================================================================
// Test Models
// =============================================================================

@PersonalBucket("test-posts")
class TestPost(
    var title: String = ""
) : PersonalActiveRecord() {

    val comments by hasMany<TestComment>()

    companion object {
        fun find(id: String): TestPost? = ActiveRecord.find(id)
        fun all(): List<TestPost> = ActiveRecord.all()
    }
}

@PersonalBucket("test-comments")
class TestComment(
    var text: String = ""
) : PersonalActiveRecord() {

    @BelongsTo(TestPost::class)
    var postId: String = ""

    val post by belongsTo<TestPost>(::postId)

    companion object {
        fun find(id: String): TestComment? = ActiveRecord.find(id)
        fun all(): List<TestComment> = ActiveRecord.all()
    }
}

@PersonalBucket("test-users")
class TestUser(
    var username: String = ""
) : PersonalActiveRecord() {

    val profile by hasOne<TestProfile>()
    val posts by hasMany<TestUserPost>("authorId")

    companion object {
        fun find(id: String): TestUser? = ActiveRecord.find(id)
        fun all(): List<TestUser> = ActiveRecord.all()
    }
}

@PersonalBucket("test-profiles")
class TestProfile(
    var bio: String = "",
    var location: String = ""
) : PersonalActiveRecord() {

    @BelongsTo(TestUser::class)
    var userId: String = ""  // Inferred from TestUser -> testUserId

    companion object {
        fun find(id: String): TestProfile? = ActiveRecord.find(id)
    }
}

@PersonalBucket("test-user-posts")
class TestUserPost(
    var title: String = ""
) : PersonalActiveRecord() {

    @BelongsTo(TestUser::class)
    var authorId: String = ""  // Explicit foreign key

    val post by belongsTo<TestPost>(::authorId)
    val comments by hasMany<TestComment>("testUserPostId")

    companion object {
        fun find(id: String): TestUserPost? = ActiveRecord.find(id)
        fun all(): List<TestUserPost> = ActiveRecord.all()
    }
}

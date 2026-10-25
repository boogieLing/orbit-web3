package wallet_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
	"usedit/internal/wallet"
)

// personalDigest 独立组装消息前缀，验证中文内容按字节而非字符计长。
func personalDigest(message string) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write([]byte("\x19Ethereum Signed Message:\n"))
	hash.Write([]byte(fmt.Sprint(len([]byte(message)))))
	hash.Write([]byte(message))
	return hash.Sum(nil)
}

// TestDigestUsesUTF8ByteLength 验证空串、英文和多字节消息使用标准字节前缀。
func TestDigestUsesUTF8ByteLength(t *testing.T) {
	// 多字节中文和表情用于暴露把字符数误作协议字节数的实现。
	for _, message := range []string{"", "hello", "连接钱包🔑\n签名仅用于登录"} {
		if !bytes.Equal(wallet.Digest(message), personalDigest(message)) {
			t.Fatalf("消息字节摘要不一致: %q", message)
		}
	}
}

// TestRecoverRealPersonalSignature 用已知密钥地址验证两种恢复位表示并拒绝被修改的消息归属。
func TestRecoverRealPersonalSignature(t *testing.T) {
	// 固定曲线私钥 1 的地址已知，避免地址计算与待测逻辑互相证明。
	key := secp256k1.PrivKeyFromBytes([]byte{1})
	want := "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"
	message := "usedit.test\n连接钱包，仅用于登录\nNonce: unique-request"
	compact := ecdsa.SignCompact(key, personalDigest(message), false)
	for _, recovery := range []byte{compact[0], compact[0] - 27} {
		raw := append(append([]byte{}, compact[1:]...), recovery)
		for _, prefix := range []string{"", "0x"} {
			address, recoverErr := wallet.Recover(message, prefix+hex.EncodeToString(raw))
			if recoverErr != nil || address != want {
				t.Fatalf("恢复签名失败: %s, %v", address, recoverErr)
			}
		}
	}
	raw := append(append([]byte{}, compact[1:]...), compact[0])
	changed, recoverErr := wallet.Recover(message+"changed", hex.EncodeToString(raw))
	if recoverErr == nil && changed == want {
		t.Fatal("修改消息后仍恢复为原账户")
	}
}

// TestRejectInvalidSignatureAndAddress 检查签名长度、十六进制、恢复位和账户地址边界。
func TestRejectInvalidSignatureAndAddress(t *testing.T) {
	// 分别覆盖编码、长度和恢复位，不把所有无效输入归结为同一种失败。
	for _, signature := range []string{"", "0xnot-hex", strings.Repeat("00", 64), strings.Repeat("00", 66), strings.Repeat("00", 64) + "02", strings.Repeat("00", 64) + "1d", strings.Repeat("00", 65)} {
		if _, recoverErr := wallet.Recover("login", signature); recoverErr == nil {
			t.Fatalf("接受了无效签名: %q", signature)
		}
	}
	for _, address := range []string{"", "0x1234", "0X" + strings.Repeat("1", 40), "0x" + strings.Repeat("g", 40), " " + "0x" + strings.Repeat("1", 40), "0x" + strings.Repeat("1", 41)} {
		if wallet.ValidAddress(address) {
			t.Fatalf("接受了无效地址: %q", address)
		}
	}
	if !wallet.ValidAddress("0x" + strings.Repeat("aB", 20)) {
		t.Fatal("拒绝了完整十六进制地址")
	}
}

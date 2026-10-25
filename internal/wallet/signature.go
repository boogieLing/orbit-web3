// Package wallet 校验普通 EVM 外部账户的个人消息签名。
package wallet

import (
	"encoding/hex"
	"fmt"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
	"log/slog"
	"regexp"
	"strings"
)

var addressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

// ValidAddress 只接受完整 EVM 地址，不进行名称解析。
func ValidAddress(address string) bool { return addressPattern.MatchString(address) }

// Digest 按 EIP-191 对 UTF-8 消息字节计算签名摘要。
func Digest(message string) []byte {
	hash := sha3.NewLegacyKeccak256()
	fmt.Fprintf(hash, "\x19Ethereum Signed Message:\n%d%s", len([]byte(message)), message)
	return hash.Sum(nil)
}

// Recover 恢复签名账户，拒绝无效恢复位和非标准签名长度。
func Recover(message, signature string) (string, error) {
	raw, decodeErr := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if decodeErr != nil || len(raw) != 65 {
		slog.Warn("wallet_signature_format")
		return "", fmt.Errorf("签名格式不正确")
	}
	// 兼容钱包返回的两种恢复位编码，不接受超出普通 EOA 签名的范围。
	recovery := raw[64]
	if recovery >= 27 {
		recovery -= 27
	}
	if recovery > 1 {
		slog.Warn("wallet_recovery_bit")
		return "", fmt.Errorf("签名恢复位不正确")
	}
	compact := append([]byte{27 + recovery}, raw[:64]...)
	public, _, recoverErr := ecdsa.RecoverCompact(compact, Digest(message))
	if recoverErr != nil {
		slog.Warn("wallet_recover", "error", recoverErr)
		return "", fmt.Errorf("签名校验失败: %w", recoverErr)
	}
	hash := sha3.NewLegacyKeccak256()
	hash.Write(public.SerializeUncompressed()[1:])
	return "0x" + hex.EncodeToString(hash.Sum(nil)[12:]), nil
}
